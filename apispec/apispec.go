package apispec

import _ "embed"

//go:embed api.yaml
var Spec []byte
