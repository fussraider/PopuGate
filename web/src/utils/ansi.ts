/**
 * Simple ANSI to HTML converter
 */
export function ansiToHtml(text: string): string {
  if (!text) return ''

  // Escape HTML
  const escaped = text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;')

  const ansiRegex = /\x1b\[([0-9;]*)m/g
  let result = escaped
  const stack: string[] = []

  result = escaped.replace(ansiRegex, (_match, code) => {
    if (code === '0' || code === '') {
      const closing = '</span>'.repeat(stack.length)
      stack.length = 0
      return closing
    }

    const codes = code.split(';')
    let style = ''
    let classes = ''

    for (const c of codes) {
      const n = parseInt(c)
      if (n === 1) style += 'font-weight:bold;'
      else if (n === 2) style += 'opacity:0.6;' // Dim
      else if (n === 3) style += 'font-style:italic;'
      else if (n >= 30 && n <= 37) classes += ` ansi-fg-${n - 30}`
      else if (n >= 90 && n <= 97) classes += ` ansi-fg-bright-${n - 90}`
      else if (n >= 40 && n <= 47) classes += ` ansi-bg-${n - 40}`
    }

    if (style || classes) {
      stack.push('</span>')
      return `<span ${classes ? `class="${classes.trim()}"` : ''} ${style ? `style="${style}"` : ''}>`
    }
    return ''
  })

  return result + '</span>'.repeat(stack.length)
}
