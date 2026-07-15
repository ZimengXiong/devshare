const escape_html = value => String(value).replace(/[&<>"']/g, char => ({
  '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
})[char])

const absolute_date = value => new Date(value).toLocaleString()

const lock_glyph = locked => locked
  ? '<svg viewBox="0 0 16 16" aria-hidden="true"><rect x="3" y="7" width="10" height="7" rx="1"/><path d="M5 7V5a3 3 0 0 1 6 0v2"/></svg>'
  : '<svg viewBox="0 0 16 16" aria-hidden="true"><rect x="3" y="7" width="10" height="7" rx="1"/><path d="M5 7V5a3 3 0 0 1 5.7-1.3"/></svg>'

let revealed_key = null

function agent_setup(key) {
  return `Set up Devshare for this machine and future agent sessions.

Server: ${location.origin}
API key: ${key.token}
Scopes: ${key.scopes.join(', ')}

Do all of the following now:

1. Install the latest Devshare CLI from https://github.com/ZimengXiong/devshare/releases/latest. Detect this machine's OS and architecture, download the matching devshare-{linux|darwin}-{amd64|arm64} binary, install it as ~/.local/bin/devshare, make it executable, and ensure ~/.local/bin is on PATH. Do not use an asset for the wrong platform.
2. Create ~/.devshare with mode 0700 and save this exact file as ~/.devshare/.env with mode 0600:
   DEVSHARE_URL=${location.origin}
   DEVSHARE_TOKEN=${key.token}
   Never commit or print this file.
3. Run: devshare auth login --url ${location.origin} --token ${key.token}
   This also configures the CLI for normal use without manually sourcing the env file.
4. Verify setup with: devshare version && devshare list
5. Create a reusable skill named devshare in this agent's native personal skills directory. Include the server URL and API key shown above directly in the skill as requested, identify ~/.devshare/.env as the durable credential source, and include these workflows:
   - devshare publish <file-or-directory>
   - devshare publish --public <file-or-directory>
   - devshare publish --keep <file-or-directory>
   - devshare publish --update <share-id-or-url> <file-or-directory>
   - devshare serve <port>
   - devshare serve --public <port>
   - devshare list
   - devshare rm <share-id>
   Explain that HTML files/directories and Markdown are supported; shares are private and temporary unless --public or --keep is supplied; --update preserves the URL; serve creates a live tunnel and must remain running.
6. Report the CLI path, ~/.devshare/.env path, CLI config path, skill path, and successful verification. If any step fails, diagnose and finish the setup instead of merely describing it.

Treat the API key above as a secret and do not repeat it in your response.`
}

async function copy_text(button, text) {
  await navigator.clipboard.writeText(text)
  const label = button.textContent
  button.textContent = 'copied'
  setTimeout(() => { button.textContent = label }, 1400)
}

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
        <button class="visibility-toggle" data-visibility="${escape_html(share.visibility)}" data-share="${escape_html(new URL(share.url).hostname)}" title="${share.visibility === 'private' ? 'Private share — click to make public' : 'Public share — click to make private'}" aria-label="${share.visibility === 'private' ? 'Private share. Make public.' : 'Public share. Make private.'}">${lock_glyph(share.visibility === 'private')}</button>
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
  revealed_key = key
  document.querySelector('#secret').innerHTML = `
    <div class="secret-card">
      <p>copy this key now. it will not be shown again.</p>
      <code class="secret-value">${escape_html(key.token)}</code>
      <div class="secret-actions">
        <button type="button" data-copy-key>copy key</button>
        <button type="button" class="secondary" data-copy-skill>copy skill setup</button>
      </div>
    </div>`
  event.target.reset()
  load_keys()
}

document.body.onclick = async event => {
  const copy_key = event.target.closest('[data-copy-key]')
  if (copy_key && revealed_key) {
    await copy_text(copy_key, revealed_key.token)
    return
  }
  const copy_skill = event.target.closest('[data-copy-skill]')
  if (copy_skill && revealed_key) {
    await copy_text(copy_skill, agent_setup(revealed_key))
    return
  }
  const visibility = event.target.closest('.visibility-toggle')
  if (visibility) {
    const next = visibility.dataset.visibility === 'private' ? 'public' : 'private'
    const warning = next === 'public'
      ? 'Make this share public? Anyone with the URL will be able to view it.'
      : 'Make this share private? Viewers will need to sign in.'
    if (!confirm(warning)) return
    const response = await fetch('/v1/dashboard/visibility', {
      method: 'post',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ share: visibility.dataset.share, visibility: next })
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
