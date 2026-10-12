package admin

import _ "embed"

//go:embed static/login.html
var loginHTML []byte

//go:embed static/index.html
var indexHTML []byte

//go:embed static/style.css
var styleCSS []byte

//go:embed static/app.js
var appJS []byte
