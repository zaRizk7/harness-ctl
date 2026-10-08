package manager

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type plistElement struct {
	XMLName xml.Name
	Text    string         `xml:",chardata"`
	Values  []plistElement `xml:",any"`
}

func serviceProgram(data []byte, label string) (string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var doc plistElement
	if err := decoder.Decode(&doc); err != nil {
		return "", fmt.Errorf("service must be a readable XML plist: %w", err)
	}
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		switch value := token.(type) {
		case xml.CharData:
			if strings.TrimSpace(string(value)) != "" {
				return "", fmt.Errorf("unexpected trailing plist content")
			}
		case xml.Comment, xml.ProcInst:
		default:
			return "", fmt.Errorf("unexpected trailing plist content")
		}
	}
	if doc.XMLName.Local != "plist" || len(doc.Values) != 1 || doc.Values[0].XMLName.Local != "dict" {
		return "", fmt.Errorf("service must contain one plist dictionary")
	}
	values := doc.Values[0].Values
	fields := map[string]plistElement{}
	if len(values)%2 != 0 {
		return "", fmt.Errorf("malformed service dictionary")
	}
	for i := 0; i < len(values); i += 2 {
		key := values[i]
		if key.XMLName.Local != "key" || len(key.Values) != 0 || key.Text == "" {
			return "", fmt.Errorf("malformed service key")
		}
		// Duplicate keys have ambiguous platform semantics, so they cannot
		// establish ownership or authorize service mutation.
		if _, exists := fields[key.Text]; exists {
			return "", fmt.Errorf("duplicate service key: %s", key.Text)
		}
		fields[key.Text] = values[i+1]
	}
	identity := fields["Label"]
	if identity.XMLName.Local != "string" || len(identity.Values) != 0 || identity.Text != label {
		return "", fmt.Errorf("service label does not match its approved filename")
	}
	if program, exists := fields["Program"]; exists {
		if program.XMLName.Local != "string" || len(program.Values) != 0 {
			return "", fmt.Errorf("invalid service program")
		}
		return program.Text, nil
	}
	args := fields["ProgramArguments"]
	if args.XMLName.Local != "array" || len(args.Values) == 0 {
		return "", fmt.Errorf("service has no launch executable")
	}
	for _, arg := range args.Values {
		if arg.XMLName.Local != "string" || len(arg.Values) != 0 {
			return "", fmt.Errorf("invalid service arguments")
		}
	}
	return args.Values[0].Text, nil
}

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
	data, err := io.ReadAll(io.LimitReader(f, e.cfg.MetadataBytes+1))
	if err != nil {
		return err
	}
	if int64(len(data)) > e.cfg.MetadataBytes {
		return fmt.Errorf("service metadata exceeds configured limit")
	}
	program, err := serviceProgram(data, label)
	if err != nil {
		return err
	}
	if !filepath.IsAbs(program) || (program != inst.Path && (inst.Root == "" || !within(inst.Root, program))) {
		return fmt.Errorf("service does not launch the selected installation: %s", path)
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
	owners := append([]string{}, p.Request.Owners...)
	if p.Request.Model == "tracked" || p.Request.Action == "migrate" {
		owners = append(owners, p.Spec.SharedClients...)
	}
	for _, owner := range owners {
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
