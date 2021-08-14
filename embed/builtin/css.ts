// a map to store recoverable css
const styleMap = new Map<string, { css?: string, href?: string }>()

export function removeCSS(url: string, recoverable?: boolean) {
  const { document } = window
  Array.from(document.head.children).forEach(el => {
    if (el.getAttribute('data-module-url') === url) {
      if (recoverable) {
        const tag = el.tagName.toLowerCase()
        if (tag === 'style') {
          styleMap.set(url, { css: el.innerHTML })
        } else if (tag === 'link') {
          const href = el.getAttribute('href')
          if (href) {
            styleMap.set(url, { href })
          }
        }
      }
      document.head.removeChild(el)
    }
  })
}

export function recoverCSS(url: string) {
  if (styleMap.has(url)) {
    applyCSS(url, styleMap.get(url)!)
  }
}

export function applyCSS(url: string, { css, href }: { css?: string, href?: string }) {
  const prevEls = Array.from(document.head.children).filter((el: any) => {
    return el.getAttribute('data-module-url') === url
  })
  let el: any
  if (css) {
    el = document.createElement('style')
    el.type = 'text/css'
    el.appendChild(document.createTextNode(css))
  } else if (href) {
    el = document.createElement('link')
    el.rel = 'stylesheet'
    el.href = href
  } else {
    throw new Error('applyCSS: missing css')
  }
  el.setAttribute('data-module-url', url)
  document.head.appendChild(el)
  if (prevEls.length > 0) {
    prevEls.forEach(el => document.head.removeChild(el))
  }
}
