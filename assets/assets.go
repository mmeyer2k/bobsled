// assets/assets.go
package assets

import _ "embed"

//go:embed bobsled@.service
var SystemdUnit []byte

//go:embed bootstrap.sh
var BootstrapScript []byte
