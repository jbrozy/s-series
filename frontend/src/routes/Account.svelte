<script>
  import { api } from '../lib/api.js';
  import { session, toast } from '../lib/state.svelte.js';

  let current = $state('');
  let next = $state('');
  let repeat = $state('');
  let error = $state('');

  async function submit(e) {
    e.preventDefault();
    error = '';
    if (next !== repeat) { error = 'Die neuen Passwörter stimmen nicht überein'; return; }
    try {
      await api.post('/auth/password', { current, new: next });
      current = next = repeat = '';
      toast('Passwort geändert');
    } catch (err) { error = err.message; }
  }
</script>

<div class="page narrow">
  <h1>Konto</h1>
  <section class="card card-pad stack" style="margin-top:16px">
    <div><span class="muted">Name</span><div><strong>{session.user.name}</strong></div></div>
    <div><span class="muted">E-Mail</span><div>{session.user.email}</div></div>
    <div><span class="muted">Rolle</span><div>{session.user.role === 'admin' ? 'Administrator' : 'Benutzer'}</div></div>
  </section>
  <form class="card card-pad stack" style="margin-top:16px" onsubmit={submit}>
    <h2>Passwort ändern</h2>
    <label><span class="lbl">Aktuelles Passwort</span><input type="password" bind:value={current} required autocomplete="current-password" /></label>
    <label><span class="lbl">Neues Passwort</span><input type="password" bind:value={next} required minlength="10" autocomplete="new-password" /></label>
    <label><span class="lbl">Wiederholen</span><input type="password" bind:value={repeat} required minlength="10" autocomplete="new-password" /></label>
    {#if error}<div class="error-box">{error}</div>{/if}
    <div class="row"><span class="spacer"></span><button class="primary">Speichern</button></div>
  </form>
</div>

<style>
  .narrow { max-width: 640px; }
</style>
