const escape_html = value => String(value).replace(/[&<>"']/g, char => ({
  '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
})[char])

const absolute_date = value => new Date(value).toLocaleString()

const lock_glyph = locked => locked
  ? '<svg viewBox="0 0 16 16" aria-hidden="true"><rect x="3" y="7" width="10" height="7" rx="1"/><path d="M5 7V5a3 3 0 0 1 6 0v2"/></svg>'
  : '<svg viewBox="0 0 16 16" aria-hidden="true"><rect x="3" y="7" width="10" height="7" rx="1"/><path d="M5 7V5a3 3 0 0 1 5.7-1.3"/></svg>'

function relative_date(value) {
  const seconds = Math.round((new Date(value).getTime() - Date.now()) / 1000)
  const units = [[31536000, 'year'], [2592000, 'month'], [86400, 'day'], [3600, 'hour'], [60, 'minute'], [1, 'second']]
  const [size, unit] = units.find(([size]) => Math.abs(seconds) >= size) || units.at(-1)
  const amount = Math.round(seconds / size)
  return new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' }).format(amount, unit)
}

document.querySelector('nav').onclick = event => {
  const page = event.target.dataset.page
  if (!page) return
  document.querySelectorAll('nav button, section').forEach(item => item.classList.remove('selected'))
  event.target.classList.add('selected')
  document.querySelector(`#${page}`).classList.add('selected')
}

async function load_shares() {
  const response = await fetch('/v1/dashboard/shares')
  if (!response.ok) throw new Error('could not load shares')
  const shares = await response.json()
  document.querySelector('#share-list').innerHTML = shares.map(share => `
    <div class="row share-row">
      <span class="share-url">
        <a href="${escape_html(share.url)}">${escape_html(share.url.replace('https://', ''))}</a>
        <button class="visibility-toggle" data-visibility="${escape_html(share.visibility)}" data-share="${escape_html(share.id)}" title="${share.visibility === 'private' ? 'Private share — click to make public' : 'Public share — click to make private'}" aria-label="${share.visibility === 'private' ? 'Private share. Make public.' : 'Public share. Make private.'}">${lock_glyph(share.visibility === 'private')}</button>
      </span>
      <span class="share-type">${escape_html(share.type)}</span>
      ${share.expiresAt ? `<button class="date-toggle muted" data-date="${escape_html(share.expiresAt)}" data-relative="false" title="Click to show relative time" aria-label="Expiry: ${escape_html(absolute_date(share.expiresAt))}. Click to show relative time.">${escape_html(absolute_date(share.expiresAt))}</button>` : '<span class="date-toggle muted no-expiry">no expiry</span>'}
      <button class="danger" data-remove="${escape_html(share.id)}">remove</button>
    </div>`).join('') || '<p>no active shares.</p>'
}

async function load_keys() {
  const response = await fetch('/v1/dashboard/tokens')
  if (!response.ok) throw new Error('could not load keys')
  const keys = await response.json()
  document.querySelector('#key-list').innerHTML = keys.map(key => `
    <div class="row key-row">
      <span>${escape_html(key.label)}</span>
      <span class="muted">${escape_html(key.scopes.join(', '))}</span>
      <span class="key-status ${key.revoked ? 'revoked' : ''}">
        ${key.bootstrap ? 'bootstrap' : key.revoked ? 'revoked' : 'active'}
      </span>
      ${key.bootstrap || key.revoked
        ? '<span></span>'
        : `<button class="danger" data-revoke="${escape_html(key.id)}">revoke</button>`}
    </div>`).join('') || '<p>no api keys.</p>'
}

document.querySelector('#new-key').onsubmit = async event => {
  event.preventDefault()
  const form = new FormData(event.target)
  const response = await fetch('/v1/dashboard/tokens', {
    method: 'post',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ label: form.get('label'), scopes: form.getAll('scope') })
  })
  if (!response.ok) return alert(await response.text())
  const key = await response.json()
  document.querySelector('#secret').textContent = `copy this key now: ${key.token}`
  event.target.reset()
  load_keys()
}

document.body.onclick = async event => {
  const visibility = event.target.closest('.visibility-toggle')
  if (visibility) {
    const next = visibility.dataset.visibility === 'private' ? 'public' : 'private'
    const warning = next === 'public'
      ? 'Make this share public? Anyone with the URL will be able to view it.'
      : 'Make this share private? Viewers will need to sign in.'
    if (!confirm(warning)) return
    const response = await fetch(`/v1/dashboard/shares/${visibility.dataset.share}/visibility`, {
      method: 'patch',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ visibility: next })
    })
    if (!response.ok) return alert(await response.text())
    load_shares()
    return
  }
  const date = event.target.closest('.date-toggle')
  if (date) {
    const relative = date.dataset.relative !== 'true'
    date.dataset.relative = relative
    date.textContent = relative ? relative_date(date.dataset.date) : absolute_date(date.dataset.date)
    date.title = relative ? 'Click to show exact time' : 'Click to show relative time'
    return
  }
  if (event.target.dataset.remove && confirm('remove this share?')) {
    const response = await fetch(`/v1/dashboard/shares/${event.target.dataset.remove}`, { method: 'delete' })
    if (!response.ok) return alert(await response.text())
    load_shares()
  }
  if (event.target.dataset.revoke && confirm('revoke this api key?')) {
    const response = await fetch(`/v1/dashboard/tokens/${event.target.dataset.revoke}`, { method: 'delete' })
    if (!response.ok) return alert(await response.text())
    load_keys()
  }
}

load_shares().catch(error => alert(error.message))
load_keys().catch(error => alert(error.message))
