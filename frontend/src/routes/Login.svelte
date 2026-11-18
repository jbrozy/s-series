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
      <svg viewBox="0 0 32 32" width="40" height="40"><rect width="32" height="32" rx="7" fill="#1f6fb8"/><path d="M9 17.5l4.5 4.5L23 11" fill="none" stroke="#fff" stroke-width="3.2" stroke-linecap="round" stroke-linejoin="round"/></svg>
      <span>S-Check</span>
    </div>
    <h1>Inhaltliche Prüfung für S-Series-Dokumentation</h1>
    <p>Exportierte Projektdateien hochladen und semantische Fehler finden, die keine Schemaprüfung sieht:</p>
    <ul>
      <li>doppelte Listeneinträge, Schritte und Vorbedingungen</li>
      <li>falsch geschriebene, doppelte oder uneinheitliche Überschriften</li>
      <li>defekte Verweise, widersprüchliche Abkürzungen und Benennungen</li>
      <li>projektspezifische Business Rules inkl. BREX-Import</li>
      <li>Körperteile in Abbildungen erkennen und normgerecht unkenntlich machen</li>
    </ul>
  </section>

  <form class="card card-pad stack" onsubmit={submit} novalidate aria-describedby={error ? 'login-error' : undefined}>
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
    min-height: 100vh; display: grid; grid-template-columns: 1.1fr 420px; gap: 60px;
    align-items: center; max-width: 1100px; margin: 0 auto; padding: 40px;
  }
  .intro { color: var(--ink-2); }
  .logo { display: flex; align-items: center; gap: 12px; font-size: 22px; font-weight: 700; color: var(--ink); margin-bottom: 28px; }
  .intro h1 { font-size: 30px; line-height: 1.2; color: var(--ink); margin-bottom: 14px; max-width: 520px; }
  .intro ul { margin: 14px 0 0; padding-left: 20px; line-height: 1.9; }
  form { padding: 28px; }
  .switch { text-align: center; }
  @media (max-width: 860px) { .wrap { grid-template-columns: 1fr; } .intro { display: none; } }
</style>
