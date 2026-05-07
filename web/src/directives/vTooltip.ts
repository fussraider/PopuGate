import type { Directive, DirectiveBinding } from 'vue'

const TOOLTIP_ATTR = 'data-tooltip-text'

let el: HTMLDivElement | null = null
let hideTimer: ReturnType<typeof setTimeout> | null = null

function getEl(): HTMLDivElement {
  if (!el) {
    el = document.createElement('div')
    el.className = 'v-tooltip'
    document.body.appendChild(el)
  }
  return el
}

function show(trigger: HTMLElement, text: string) {
  if (hideTimer) { clearTimeout(hideTimer); hideTimer = null }
  const tip = getEl()
  tip.textContent = text
  tip.classList.add('visible')
  tip.classList.remove('hidden')

  const r = trigger.getBoundingClientRect()
  const tw = tip.offsetWidth
  const th = tip.offsetHeight
  const left = Math.max(8, Math.min(r.left + r.width / 2 - tw / 2, window.innerWidth - tw - 8))
  let top = r.top - th - 8 + window.scrollY

  // flip below if clipped by viewport top
  if (r.top - th - 8 < 0) {
    top = r.bottom + 8 + window.scrollY
  }

  tip.style.left = `${left}px`
  tip.style.top = `${top}px`
}

function hide() {
  if (!el) return
  el.classList.add('hidden')
  el.classList.remove('visible')
  hideTimer = setTimeout(() => { if (el) el.textContent = '' }, 200)
}

function onEnter(this: HTMLElement) {
  const text = this.getAttribute(TOOLTIP_ATTR)
  if (text) show(this, text)
}

const vTooltip: Directive = {
  mounted(node: HTMLElement, binding: DirectiveBinding<string>) {
    node.setAttribute(TOOLTIP_ATTR, binding.value ?? '')
    node.addEventListener('mouseenter', onEnter)
    node.addEventListener('mouseleave', hide)
    node.addEventListener('mousedown', hide)
  },
  updated(node: HTMLElement, binding: DirectiveBinding<string>) {
    node.setAttribute(TOOLTIP_ATTR, binding.value ?? '')
  },
  beforeUnmount(node: HTMLElement) {
    node.removeEventListener('mouseenter', onEnter)
    node.removeEventListener('mouseleave', hide)
    node.removeEventListener('mousedown', hide)
    hide()
  },
}

export default vTooltip
