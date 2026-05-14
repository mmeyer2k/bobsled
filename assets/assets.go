// assets/assets.go
package assets

import _ "embed"

//go:embed bobsled@.service
var SystemdUnit []byte

//go:embed registry.service
var RegistryUnit []byte

//go:embed bootstrap.sh
var BootstrapScript []byte

//go:embed registry-config.json.tmpl
var RegistryConfigTemplate string

//go:embed registries.conf.tmpl
var RegistriesConfTemplate string
