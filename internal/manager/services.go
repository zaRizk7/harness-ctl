package manager

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func (e *engine) validateService(inst installation, path string) error {
	s, err := specFor(inst.Harness)
	if err != nil {
		return err
	}
	label := strings.TrimSuffix(filepath.Base(path), ".plist")
	if !contains(s.LaunchLabels, label) || filepath.Dir(path) != filepath.Join(e.cfg.Home, "Library/LaunchAgents") {
		return fmt.Errorf("unverified service ownership")
	}
	if err = validateOwnedPath(filepath.Dir(path), path); err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	decoder := xml.NewDecoder(io.LimitReader(f, e.cfg.MetadataBytes))
	owned := false
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("service must be a readable XML plist: %w", err)
		}
		if start, ok := token.(xml.StartElement); ok && start.Name.Local == "string" {
			var value string
			if err = decoder.DecodeElement(&value, &start); err != nil {
				return err
			}
			if value == inst.Path || (inst.Root != "" && filepath.IsAbs(value) && within(inst.Root, value)) {
				owned = true
			}
		}
	}
	if !owned {
		return fmt.Errorf("service does not reference the selected installation: %s", path)
	}
	return nil
}

func (e *engine) stopServices(ctx context.Context, inst installation, record *operationRecord) error {
	for _, path := range inst.ServicePaths {
		if err := e.validateService(inst, path); err != nil {
			return err
		}
		label := strings.TrimSuffix(filepath.Base(path), ".plist")
		domain := "gui/" + strconv.Itoa(os.Getuid())
		out, err := e.run.Run(ctx, command{Path: "/bin/launchctl", Args: []string{"print", domain + "/" + label}, Description: "Check loaded service " + label})
		if err != nil {
			if strings.Contains(out, "Could not find service") {
				continue
			}
			return fmt.Errorf("cannot determine service status: %w", err)
		}
		record.Services = append(record.Services, path)
		if err = e.saveRecord(*record); err != nil {
			return err
		}
		if _, err = e.run.Run(ctx, command{Path: "/bin/launchctl", Args: []string{"bootout", domain, path}, Description: "Stop owned service " + label}); err != nil {
			return err
		}
	}
	return nil
}

func (e *engine) restartServices(ctx context.Context, paths []string) error {
	for _, path := range paths {
		// Interrupted-operation journals are editable. Re-establish service
		// ownership against a native or registered install before bootstrapping.
		installs, err := e.discover(ctx)
		if err != nil {
			return err
		}
		valid := false
		for _, inst := range installs {
			if contains(inst.ServicePaths, path) && e.validateService(inst, path) == nil {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("cannot re-establish service ownership for %s", path)
		}
		label := strings.TrimSuffix(filepath.Base(path), ".plist")
		domain := "gui/" + strconv.Itoa(os.Getuid())
		out, checkErr := e.run.Run(ctx, command{Path: "/bin/launchctl", Args: []string{"print", domain + "/" + label}, Description: "Check service before restoration"})
		if checkErr == nil {
			continue
		}
		if !strings.Contains(out, "Could not find service") {
			return checkErr
		}
		if _, err = e.run.Run(ctx, command{Path: "/bin/launchctl", Args: []string{"bootstrap", "gui/" + strconv.Itoa(os.Getuid()), path}, Description: "Restore previously loaded service"}); err != nil {
			return err
		}
	}
	return nil
}

func (e *engine) checkProcesses(ctx context.Context, p *plan) error {
	out, err := e.run.Run(ctx, command{Path: "/bin/ps", Args: []string{"-axo", "pid=,command="}, Description: "Check affected running clients"})
	if err != nil {
		return err
	}
	var needles []string
	if p.Install.Path != "" {
		needles = append(needles, p.Install.Path)
	}
	if p.Install.Root != "" {
		needles = append(needles, p.Install.Root)
	}
	installs, err := e.discover(ctx)
	if err != nil {
		return err
	}
	for _, inst := range installs {
		if contains(p.Request.Owners, inst.Harness) {
			needles = append(needles, inst.Path)
			if inst.Root != "" {
				needles = append(needles, inst.Root)
			}
		}
	}
	for _, owner := range append(append([]string{}, p.Request.Owners...), p.Spec.SharedClients...) {
		if owner == "Codex desktop / IDE" {
			needles = append(needles, "/Codex.app/")
		}
		if owner == "Claude desktop / IDE" {
			needles = append(needles, "/Claude.app/")
		}
	}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil || pid == os.Getpid() {
			continue
		}
		for _, needle := range needles {
			if strings.Contains(strings.Join(fields[1:], " "), needle) {
				return fmt.Errorf("affected client is running (PID %d). Close it, then create a fresh preview", pid)
			}
		}
	}
	return nil
}
