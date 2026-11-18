<script>
  import { api } from '../lib/api.js';
  import { session, toast } from '../lib/state.svelte.js';
  import { go } from '../lib/router.svelte.js';
  import ApiTokens from '../components/ApiTokens.svelte';

  let { forced = false } = $props();
  let name = $state(session.user.name);
  let nameError = $state('');
  let current = $state('');
  let next = $state('');
  let repeat = $state('');
  let error = $state('');
  let busy = $state(false);

  async function saveName(e) {
    e.preventDefault();
    nameError = '';
    if (!name.trim()) return (nameError = 'Bitte einen Namen angeben.');
    try {
      session.user = await api.patch('/auth/me', { name });
      toast('Name gespeichert');
    } catch (err) { nameError = err.message; }
  }

  async function submit(e) {
    e.preventDefault();
    error = '';
    if (next.length < 10) return (error = 'Das neue Passwort muss mindestens 10 Zeichen lang sein.');
    if (next !== repeat) return (error = 'Die neuen Passwörter stimmen nicht überein.');
    busy = true;
    try {
      const res = await api.post('/auth/password', { current, new: next });
      current = next = repeat = '';
      session.user = res.user;
      toast(res.endedSessions ? `Passwort geändert. ${res.endedSessions} andere Anmeldung(en) beendet.` : 'Passwort geändert');
      if (forced) go('/');
    } catch (err) { error = err.message; } finally { busy = false; }
  }
</script>

<div class="page narrow">
  {#if forced}
    <div class="notice" role="alert">
      <h1>Willkommen, {session.user.name}</h1>
      <p>Ihr Konto wurde von einer Administratorin bzw. einem Administrator angelegt oder das Passwort wurde zurückgesetzt.
        Bitte legen Sie jetzt ein eigenes Passwort fest. Als „aktuelles Passwort“ geben Sie das erhaltene Startpasswort ein.</p>
    </div>
  {:else}
    <h1>Konto</h1>
    <form class="card card-pad stack" onsubmit={saveName} novalidate>
      <h2>Profil</h2>
      <label><span class="lbl">Name</span><input bind:value={name} autocomplete="name" /></label>
      <div><span class="lbl">E-Mail</span><span>{session.user.email}</span><span class="hint mailhint">Ändern kann sie ein Administrator.</span></div>
      <div><span class="lbl">Rolle</span><span>{session.user.role === 'admin' ? 'Administrator' : 'Benutzer'}</span></div>
      {#if nameError}<div class="error-box" role="alert">{nameError}</div>{/if}
      <div class="row"><span class="spacer"></span><button class="primary" disabled={name === session.user.name}>Speichern</button></div>
    </form>
  {/if}

  <form class="card card-pad stack" onsubmit={submit} novalidate>
    <h2>{forced ? 'Eigenes Passwort festlegen' : 'Passwort ändern'}</h2>
    <label><span class="lbl">{forced ? 'Startpasswort' : 'Aktuelles Passwort'}</span><input type="password" bind:value={current} autocomplete="current-password" /></label>
    <label><span class="lbl">Neues Passwort</span><input type="password" bind:value={next} autocomplete="new-password" aria-describedby="pw-rule" />
      <span class="hint" id="pw-rule">Mindestens 10 Zeichen</span></label>
    <label><span class="lbl">Neues Passwort wiederholen</span><input type="password" bind:value={repeat} autocomplete="new-password" /></label>
    {#if error}<div class="error-box" role="alert">{error}</div>{/if}
    {#if !forced}<p class="hint">Nach der Änderung werden Sie auf allen anderen Geräten abgemeldet.</p>{/if}
    <div class="row"><span class="spacer"></span><button class="primary" disabled={busy}>{forced ? 'Passwort festlegen und weiter' : 'Passwort ändern'}</button></div>
  </form>
  {#if !forced}<ApiTokens />{/if}
</div>

<style>
  .mailhint { display: block; }
  .narrow { max-width: 880px; display: flex; flex-direction: column; gap: 16px; }
  .lbl { display: block; font-size: 12.5px; font-weight: 600; color: var(--ink-2); margin-bottom: 4px; }
  .notice { background: var(--accent-soft); border: 1px solid var(--accent); border-radius: var(--r-2); padding: 18px 20px; display: flex; flex-direction: column; gap: 8px; }
</style>
