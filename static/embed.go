package static

import "embed"

//go:embed css/*.css js/*.js img/*.ico
var FS embed.FS
