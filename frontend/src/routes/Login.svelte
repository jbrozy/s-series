<script>
  import { onMount } from 'svelte';
  import { api } from '../lib/api.js';
  import { session } from '../lib/state.svelte.js';
  import { go, route } from '../lib/router.svelte.js';
  import { desktop, connectServer, serverHost, insecure } from '../lib/desktop.svelte.js';

  let mode = $state('login');
  let email = $state('');
  let name = $state('');
  let password = $state('');
  let error = $state('');
  let busy = $state(false);
  // desktop version: no registration link before a server has answered
  let cfg = $state({ allowSignup: !desktop.on, firstUser: false });

  // desktop version: which S-Check server to sign in to
  let serverInput = $state(desktop.server);
  let serverError = $state('');
  let checking = $state(false);
  let serverField = $state();
  let emailField = $state();
  const serverOk = $derived(desktop.server !== '' && serverInput.trim() === desktop.server && !serverError);

  async function loadConfig() {
    cfg = await api.get('/auth/config').catch(() => ({ allowSignup: false, firstUser: false }));
    mode = cfg.firstUser ? 'register' : 'login';
  }

  onMount(async () => {
    const wantServer = desktop.wantServer;
    desktop.wantServer = false;
    if (desktop.on && !desktop.server) {
      serverField?.focus();
      return;
    }
    await loadConfig();
    if (desktop.on && (wantServer || 'server' in route.query) && !desktop.locked) serverField?.focus();
    else emailField?.focus();
  });

  async function checkServer() {
    if (!desktop.on || desktop.locked) return true;
    const v = serverInput.trim();
    if (!v) { serverError = 'Bitte die Adresse des S-Check-Servers angeben, z. B. s-check.firma.de.'; return false; }
    if (v === desktop.server && !serverError) return true;
    checking = true;
    serverError = '';
    try {
      await connectServer(v);
      serverInput = desktop.server;
      await loadConfig();
      return true;
    } catch (err) {
      serverError = err.message;
      return false;
    } finally {
      checking = false;
    }
  }

  async function submit(e) {
    e.preventDefault();
    error = '';
    if (!(await checkServer())) { serverField?.focus(); return; }
    if (mode === 'register' && !name.trim()) { error = 'Bitte geben Sie Ihren Namen ein.'; return; }
    if (!/^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(email.trim())) { error = 'Bitte geben Sie eine gültige E-Mail-Adresse ein.'; return; }
    if (!password) { error = 'Bitte geben Sie Ihr Passwort ein.'; return; }
    if (mode === 'register' && password.length < 10) { error = `Das Passwort muss mindestens 10 Zeichen lang sein (aktuell ${password.length}).`; return; }
    busy = true;
    try {
      session.user = mode === 'login'
        ? await api.post('/auth/login', { email, password })
        : await api.post('/auth/register', { email, name, password });
      go('/');
    } catch (err) {
      error = err.message;
    } finally {
      busy = false;
    }
  }
</script>

<main class="wrap">
  <header class="lhead">
    <h1>S-Check</h1>
    <p>Inhaltliche Prüfung für S-Series-Dokumentation</p>
  </header>

  <form class="card stack" onsubmit={submit} novalidate aria-describedby={error ? 'login-error' : undefined}>
    <h2>{mode === 'login' ? 'Anmelden' : cfg.firstUser ? 'Ersten Administrator anlegen' : 'Konto erstellen'}</h2>
    {#if desktop.on}
      {#if desktop.locked}
        <p class="srv"><span class="lbl">Server</span> <span class="mono">{serverHost()}</span> <span class="hint">(von Ihrer IT vorgegeben)</span></p>
      {:else}
        <label>
          <span class="lbl">Server</span>
          <input bind:this={serverField} bind:value={serverInput} list="recent-servers" inputmode="url" autocomplete="off" spellcheck="false"
            placeholder="s-check.firma.de" onchange={checkServer} oninput={() => (serverError = '')}
            aria-invalid={!!serverError} aria-describedby={serverError ? 'server-error' : 'server-hint'} />
          {#if desktop.recent.length}
            <datalist id="recent-servers">{#each desktop.recent as r (r)}<option value={r}></option>{/each}</datalist>
          {/if}
          {#if serverError}
            <span class="field-error" id="server-error" role="alert">{serverError}</span>
          {:else if checking}
            <span class="hint" id="server-hint" role="status">Server wird geprüft …</span>
          {:else if serverOk}
            <span class="hint ok" id="server-hint" role="status">Verbunden mit {serverHost()}</span>
          {:else}
            <span class="hint" id="server-hint">Adresse des S-Check-Servers Ihrer Organisation. Ohne Angabe wird HTTPS verwendet.</span>
          {/if}
          {#if insecure(serverInput) && !serverError}
            <span class="warn-line">Achtung: unverschlüsselte Verbindung (http). Passwort und Daten werden lesbar übertragen.</span>
          {/if}
        </label>
      {/if}
    {/if}
    {#if mode === 'register'}
      <label><span class="lbl">Name</span><input bind:value={name} required autocomplete="name" /></label>
    {/if}
    <label><span class="lbl">E-Mail</span><input type="email" bind:this={emailField} bind:value={email} required autocomplete="email" /></label>
    <label>
      <span class="lbl">Passwort</span>
      <input type="password" bind:value={password} required minlength={mode === 'register' ? 10 : 1}
        autocomplete={mode === 'login' ? 'current-password' : 'new-password'} />
      {#if mode === 'register'}<span class="hint" id="pw-hint">Mindestens 10 Zeichen</span>{/if}
    </label>
    {#if error}<div class="error-box" id="login-error" role="alert">{error}</div>{/if}
    <button class="primary" type="submit" disabled={busy || checking}>{mode === 'login' ? 'Anmelden' : 'Registrieren'}</button>
    {#if cfg.allowSignup && !cfg.firstUser}
      <p class="hint switch">
        {#if mode === 'login'}
          Noch kein Konto? <a href="#/login" onclick={(e) => { e.preventDefault(); mode = 'register'; error = ''; }}>Registrieren</a>
        {:else}
          Bereits registriert? <a href="#/login" onclick={(e) => { e.preventDefault(); mode = 'login'; error = ''; }}>Anmelden</a>
        {/if}
      </p>
    {/if}
  </form>
  {#if desktop.on && desktop.version}<p class="hint ver">S-Check Desktop {desktop.version}</p>{/if}
</main>

<style>
  .wrap { min-height: 100vh; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: var(--s-4); padding: var(--s-5) var(--s-3); }
  .lhead { text-align: center; }
  .lhead h1 { font-size: 28px; }
  .lhead p { color: var(--muted); }
  form { width: min(400px, 100%); padding: var(--s-5); }
  form h2 { font-size: var(--t-xl); }
  form .primary { min-height: 40px; }
  .switch { text-align: center; }
  .srv { display: flex; flex-wrap: wrap; gap: var(--s-1) var(--s-2); align-items: baseline; margin: 0; }
  .hint.ok { color: var(--ok); }
  .field-error { display: block; margin-top: var(--s-1); color: var(--error); font-size: var(--t-sm); }
  .warn-line { display: block; margin-top: var(--s-1); color: var(--warning); font-size: var(--t-sm); font-weight: 600; }
  .ver { text-align: center; }
</style>
