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
	cssLoader = `import { applyCSS } from '/builtin:css.ts'
applyCSS('%s', {css: %s})
`
)

var (
	defaultModuleExts = []string{".ts", ".mjs", ".js", ".tsx", ".jsx"}
)
