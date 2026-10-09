package manager

import (
	"encoding/json"
	"github.com/zaRizk7/harness-ctl/internal/stateconfig"
)

// marshalJSON binds serialization of native registrations and manifests. Tests
// can fail the boundary without changing validated data or ownership decisions.
var marshalJSON = json.Marshal

// marshalJSONIndent binds human-editable request/configuration serialization.
// Failed serialization must stop publication and editor startup.
var marshalJSONIndent = json.MarshalIndent

// encodeConfig binds native-format encoding before approved state publication.
var encodeConfig = stateconfig.Encode
