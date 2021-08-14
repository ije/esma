package server

const (
	defaultIndexHtml = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
</head>
<body>
  <script src="%s"></script>
</body>
</html>
`
	cssLoader = `
const el = document.createElement('style')
el.type = 'text/css'
el.setAttribute('data-module-url', "%s")
el.appendChild(document.createTextNode(%s))
document.head.appendChild(el)
`
)

var (
	defaultModuleExts = []string{".ts", ".mjs", ".js", ".tsx", ".jsx"}
)
