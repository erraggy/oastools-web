package static

import "embed"

//go:embed css/*.css js/*.js img/*.ico img/logo-*.png img/favicon-*.png img/apple-touch-icon.png fonts/*.woff2
var FS embed.FS
