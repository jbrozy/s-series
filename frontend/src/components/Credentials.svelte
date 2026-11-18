<script>
  // Shows one-time login credentials with a copy action.
  let { name, email, password, note = '' } = $props();
  let copied = $state(false);

  const text = $derived(
    `Zugang zu S-Check\nAdresse: ${location.origin}\nE-Mail: ${email}\nStartpasswort: ${password}\nBeim ersten Anmelden wird ein eigenes Passwort festgelegt.`
  );

  async function copy() {
    try {
      await navigator.clipboard.writeText(text);
    } catch {
      // http without secure context: fallback
      const ta = document.createElement('textarea');
      ta.value = text;
      document.body.appendChild(ta);
      ta.select();
      document.execCommand('copy');
      ta.remove();
    }
    copied = true;
    setTimeout(() => (copied = false), 2500);
  }
</script>

<div class="cred" role="status">
  <strong>Zugangsdaten für {name}</strong>
  <dl>
    <dt>E-Mail</dt><dd class="mono">{email}</dd>
    <dt>Startpasswort</dt><dd class="mono pw">{password}</dd>
  </dl>
  <p class="hint">{note || 'Das Passwort wird nur jetzt angezeigt. Bitte sicher weitergeben. Beim ersten Anmelden muss ein eigenes Passwort festgelegt werden.'}</p>
  <button class="small" onclick={copy}>{copied ? 'Kopiert ✓' : 'Zugangsdaten kopieren'}</button>
</div>

<style>
  .cred { background: var(--ok-soft); border: 1px solid var(--ok); border-radius: var(--r-1); padding: 12px 14px; display: flex; flex-direction: column; gap: 8px; align-items: flex-start; }
  dl { margin: 0; display: grid; grid-template-columns: auto 1fr; gap: 2px 12px; }
  dt { color: var(--muted); font-size: 13px; }
  dd { margin: 0; }
  .pw { font-size: 15px; font-weight: 700; letter-spacing: .04em; }
</style>
