<script>
  import { onMount } from 'svelte';
  import { api } from '../lib/api.js';
  import { session } from '../lib/state.svelte.js';
  import { go } from '../lib/router.svelte.js';

  let mode = $state('login');
  let email = $state('');
  let name = $state('');
  let password = $state('');
  let error = $state('');
  let busy = $state(false);
  let cfg = $state({ allowSignup: true, firstUser: false });

  onMount(async () => {
    cfg = await api.get('/auth/config').catch(() => cfg);
    if (cfg.firstUser) mode = 'register';
  });

  async function submit(e) {
    e.preventDefault();
    error = '';
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
  <section class="intro">
    <div class="logo">
      <span>S-Check</span>
    </div>
    <h1>Inhaltliche Prüfung für S-Series-Dokumentation</h1>
    <p class="lead">Exportierte Projektdateien hochladen und semantische Fehler finden, die keine Schemaprüfung sieht.</p>
    <h2 class="scope">Was S-Check prüft</h2>
    <ol>
      <li>doppelte Listeneinträge, Schritte und Vorbedingungen</li>
      <li>falsch geschriebene, doppelte oder uneinheitliche Überschriften</li>
      <li>defekte Verweise, widersprüchliche Abkürzungen und Benennungen</li>
      <li>projektspezifische Business Rules inkl. BREX-Import</li>
      <li>Körperteile in Abbildungen erkennen und normgerecht unkenntlich machen</li>
    </ol>
  </section>

  <form class="card stack" onsubmit={submit} novalidate aria-describedby={error ? 'login-error' : undefined}>
    <h2>{mode === 'login' ? 'Anmelden' : cfg.firstUser ? 'Ersten Administrator anlegen' : 'Konto erstellen'}</h2>
    {#if mode === 'register'}
      <label><span class="lbl">Name</span><input bind:value={name} required autocomplete="name" /></label>
    {/if}
    <label><span class="lbl">E-Mail</span><input type="email" bind:value={email} required autocomplete="email" /></label>
    <label>
      <span class="lbl">Passwort</span>
      <input type="password" bind:value={password} required minlength={mode === 'register' ? 10 : 1}
        autocomplete={mode === 'login' ? 'current-password' : 'new-password'} />
      {#if mode === 'register'}<span class="hint" id="pw-hint">Mindestens 10 Zeichen</span>{/if}
    </label>
    {#if error}<div class="error-box" id="login-error" role="alert">{error}</div>{/if}
    <button class="primary" type="submit" disabled={busy}>{mode === 'login' ? 'Anmelden' : 'Registrieren'}</button>
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
</main>

<style>
  .wrap {
    min-height: 100vh; display: grid; grid-template-columns: minmax(0, 1.15fr) 400px; gap: var(--s-8);
    align-items: center; max-width: 1120px; margin: 0 auto; padding: var(--s-7) var(--s-6);
  }
  .intro { color: var(--ink-2); border-top: 3px solid var(--rule); padding-top: var(--s-4); }
  .logo { display: flex; align-items: center; gap: var(--s-3); font-family: var(--font-head); font-size: 24px; font-weight: 600; color: var(--ink); margin-bottom: var(--s-6); }
  .intro h1 { font-size: 40px; line-height: 1.1; color: var(--ink); margin-bottom: var(--s-4); max-width: 560px; }
  .lead { font-size: var(--t-lg); max-width: 520px; margin-bottom: var(--s-5); }
  .intro ol { list-style: none; counter-reset: c; margin: var(--s-2) 0 0; padding: 0; border-top: 1px solid var(--line-strong); max-width: 560px; }
  .intro li { counter-increment: c; display: grid; grid-template-columns: 40px 1fr; padding: var(--s-2) 0; border-bottom: 1px solid var(--line); }
  .intro li::before { content: counter(c); font-family: var(--font-head); font-weight: 600; color: var(--accent); }
  .scope { font-size: var(--t-lg); margin-bottom: var(--s-1); }
  form { padding: var(--s-5); border-top: 4px solid var(--accent); box-shadow: var(--e-1); }
  form h2 { margin-bottom: var(--s-1); }
  form .primary { justify-content: center; min-height: 40px; }
  .switch { text-align: center; }
  @media (max-width: 900px) {
    .wrap { grid-template-columns: minmax(0, 1fr); gap: var(--s-5); padding: var(--s-5) var(--s-4); align-content: start; }
    .intro h1 { font-size: 28px; }
    .intro ol, .lead, .scope { display: none; }
    .logo { margin-bottom: var(--s-3); }
  }
</style>
