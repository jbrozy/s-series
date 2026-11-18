<script>
  import { onMount } from 'svelte';
  import { api, fmt, labels } from '../lib/api.js';
  import { href } from '../lib/router.svelte.js';
  import { session, toast, toastError } from '../lib/state.svelte.js';
  import Modal from './Modal.svelte';
  import Credentials from './Credentials.svelte';

  let users = $state([]);
  let general = $state({ allowSignup: true });
  let filter = $state('');
  let status = $state('all');

  // dialogs
  let createOpen = $state(false);
  let form = $state({ name: '', email: '', role: 'user', mode: 'temp', password: '' });
  let formError = $state('');
  let created = $state(null);
  let editing = $state(null);
  let editProjects = $state([]);
  let resetTarget = $state(null);
  let resetResult = $state(null);
  let deleteTarget = $state(null);
  let busy = $state(false);

  const shown = $derived(users.filter((u) => {
    if (status === 'admins' && u.role !== 'admin') return false;
    if (status === 'disabled' && !u.disabled) return false;
    if (status === 'pending' && !u.mustChangePassword) return false;
    if (!filter) return true;
    return (u.name + ' ' + u.email).toLowerCase().includes(filter.toLowerCase());
  }));

  onMount(async () => {
    try {
      [users, general] = await Promise.all([api.get('/admin/users'), api.get('/admin/settings/general')]);
    } catch (e) { toastError(e); }
  });

  async function reload() {
    users = await api.get('/admin/users').catch(() => users);
  }

  async function toggleSignup() {
    try {
      general = await api.put('/admin/settings/general', { ...general, allowSignup: !general.allowSignup });
      toast(general.allowSignup ? 'Selbstregistrierung aktiviert' : 'Selbstregistrierung deaktiviert');
    } catch (e) { toastError(e); }
  }

  function openCreate() {
    form = { name: '', email: '', role: 'user', mode: 'temp', password: '' };
    formError = '';
    created = null;
    createOpen = true;
  }

  async function create(e) {
    e.preventDefault();
    formError = '';
    if (!form.name.trim()) return (formError = 'Bitte einen Namen angeben.');
    if (!/^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(form.email.trim())) return (formError = 'Bitte eine gültige E-Mail-Adresse angeben.');
    if (form.mode === 'set' && form.password.length < 10) return (formError = 'Das Passwort muss mindestens 10 Zeichen lang sein.');
    busy = true;
    try {
      const res = await api.post('/admin/users', {
        name: form.name, email: form.email, role: form.role, password: form.mode === 'set' ? form.password : ''
      });
      created = { name: res.user.name, email: res.user.email, password: res.tempPassword || form.password };
      await reload();
    } catch (err) {
      formError = err.message;
    } finally {
      busy = false;
    }
  }

  async function openEdit(u) {
    editing = { ...u };
    editProjects = [];
    formError = '';
    editProjects = await api.get(`/admin/users/${u.id}/projects`).catch(() => []);
  }

  async function saveEdit(e) {
    e.preventDefault();
    formError = '';
    busy = true;
    try {
      const orig = users.find((x) => x.id === editing.id);
      const patch = {};
      for (const k of ['name', 'email', 'role']) if (editing[k] !== orig[k]) patch[k] = editing[k];
      if (Object.keys(patch).length) await api.patch(`/admin/users/${editing.id}`, patch);
      if (editing.id === session.user.id && patch.name) session.user.name = patch.name;
      editing = null;
      toast('Benutzer gespeichert');
      await reload();
    } catch (err) {
      formError = err.message;
    } finally {
      busy = false;
    }
  }

  async function setDisabled(u, disabled) {
    try {
      const upd = await api.patch(`/admin/users/${u.id}`, { disabled });
      Object.assign(u, upd);
      toast(disabled ? `${u.name} gesperrt und abgemeldet` : `${u.name} entsperrt`);
    } catch (e) { toastError(e); }
  }

  async function changeRole(u, role) {
    try {
      Object.assign(u, await api.patch(`/admin/users/${u.id}`, { role }));
    } catch (e) {
      toastError(e);
      await reload();
    }
  }

  async function doReset() {
    busy = true;
    try {
      const res = await api.post(`/admin/users/${resetTarget.id}/reset-password`);
      resetResult = { name: resetTarget.name, email: resetTarget.email, password: res.tempPassword };
      await reload();
    } catch (e) { toastError(e); } finally { busy = false; }
  }

  async function doDelete() {
    busy = true;
    try {
      const res = await api.del(`/admin/users/${deleteTarget.id}`);
      toast(res.projectsHandedOver
        ? `${deleteTarget.name} gelöscht. ${res.projectsHandedOver} Projekt(e) an Sie übergeben.`
        : `${deleteTarget.name} gelöscht`);
      deleteTarget = null;
      await reload();
    } catch (e) { toastError(e); } finally { busy = false; }
  }
</script>

<section class="card card-pad row signup">
  <div class="grow">
    <h2 class="h3">Selbstregistrierung</h2>
    <p class="hint">
      {general.allowSignup
        ? 'Jede Person mit Zugriff auf die Adresse kann ein Konto anlegen. Neue Konten sehen erst Projekte, wenn sie hinzugefügt werden.'
        : 'Nur Administratoren legen Konten an, hier oder direkt beim Hinzufügen zu einem Projekt.'}
    </p>
  </div>
  <label class="row switch">
    <input type="checkbox" checked={general.allowSignup} onchange={toggleSignup} />
    {general.allowSignup ? 'erlaubt' : 'deaktiviert'}
  </label>
</section>

<div class="row wrap bar">
  <input class="search" bind:value={filter} placeholder="Name oder E-Mail suchen …" aria-label="Benutzer suchen" />
  {#each [['all', 'Alle'], ['admins', 'Administratoren'], ['pending', 'Passwort ausstehend'], ['disabled', 'Gesperrt']] as [k, l]}
    <button class="chip" class:active={status === k} aria-pressed={status === k} onclick={() => (status = k)}>{l}</button>
  {/each}
  <span class="spacer"></span>
  <button class="primary" onclick={openCreate}>+ Benutzer anlegen</button>
</div>

<section class="card">
  <table>
    <thead><tr><th>Name</th><th>E-Mail</th><th>Rolle</th><th class="right">Projekte</th><th>Letzte Anmeldung</th><th><span class="sr-only">Aktionen</span></th></tr></thead>
    <tbody>
      {#each shown as u (u.id)}
        <tr class:off={u.disabled}>
          <td>
            <strong>{u.name}</strong>{#if u.id === session.user.id} <span class="hint">(Sie)</span>{/if}
            <div class="row wrap tags">
              {#if u.disabled}<span class="badge">gesperrt</span>{/if}
              {#if u.mustChangePassword}<span class="badge warning" title="Muss beim nächsten Anmelden ein eigenes Passwort festlegen">Startpasswort</span>{/if}
              {#if u.createdBy}<span class="hint">angelegt von {u.createdBy}</span>{/if}
            </div>
          </td>
          <td>{u.email}</td>
          <td>
            <select value={u.role} aria-label={`Rolle von ${u.name}`} disabled={u.id === session.user.id} onchange={(e) => changeRole(u, e.currentTarget.value)} style="width:auto">
              <option value="user">Benutzer</option><option value="admin">Administrator</option>
            </select>
          </td>
          <td class="right">{u.projects}</td>
          <td class="muted nowrap">{u.lastLoginAt ? fmt.date(u.lastLoginAt) : 'noch nie'}</td>
          <td class="right nowrap"><div class="actions">
            <button class="small" onclick={() => openEdit(u)}>Bearbeiten</button>
            {#if u.id !== session.user.id}
              <button class="small" title="Neues Startpasswort erzeugen und alle Anmeldungen beenden" onclick={() => { resetTarget = u; resetResult = null; }}>Passwort</button>
              <button class="small" title={u.disabled ? 'Anmelden wieder erlauben' : 'Anmelden verhindern und abmelden'} onclick={() => setDisabled(u, !u.disabled)}>{u.disabled ? 'Entsperren' : 'Sperren'}</button>
              <button class="small danger" onclick={() => (deleteTarget = u)}>Löschen</button>
            {/if}
          </div></td>
        </tr>
      {:else}
        <tr><td colspan="6" class="empty">Keine Benutzer gefunden.</td></tr>
      {/each}
    </tbody>
  </table>
</section>

{#if createOpen}
  <Modal title="Benutzer anlegen" onclose={() => (createOpen = false)}>
    {#if created}
      <div class="stack">
        <Credentials {...created} />
        <p class="hint">Die Person sieht Projekte, sobald sie unter <em>Projekt → Mitglieder</em> hinzugefügt wird.</p>
        <div class="row"><span class="spacer"></span>
          <button onclick={openCreate}>Weitere Person anlegen</button>
          <button class="primary" onclick={() => (createOpen = false)}>Fertig</button>
        </div>
      </div>
    {:else}
      <form class="stack" onsubmit={create} novalidate>
        <label><span class="lbl">Name</span><input bind:value={form.name} autocomplete="off" /></label>
        <label><span class="lbl">E-Mail</span><input type="email" bind:value={form.email} autocomplete="off" /></label>
        <label><span class="lbl">Rolle</span>
          <select bind:value={form.role}><option value="user">Benutzer</option><option value="admin">Administrator</option></select>
          <span class="hint">Administratoren verwalten Benutzer, Modelle und sehen alle Projekte.</span>
        </label>
        <fieldset class="stack">
          <legend class="lbl">Passwort</legend>
          <label class="row opt"><input type="radio" bind:group={form.mode} value="temp" /> Startpasswort erzeugen (empfohlen)</label>
          <label class="row opt"><input type="radio" bind:group={form.mode} value="set" /> Selbst festlegen</label>
          {#if form.mode === 'set'}<input type="text" bind:value={form.password} aria-label="Startpasswort" placeholder="mindestens 10 Zeichen" autocomplete="off" />{/if}
          <span class="hint">In beiden Fällen legt die Person beim ersten Anmelden ein eigenes Passwort fest.</span>
        </fieldset>
        {#if formError}<div class="error-box" role="alert">{formError}</div>{/if}
        <div class="row"><span class="spacer"></span>
          <button type="button" onclick={() => (createOpen = false)}>Abbrechen</button>
          <button class="primary" disabled={busy}>Anlegen</button>
        </div>
      </form>
    {/if}
  </Modal>
{/if}

{#if editing}
  <Modal title="Benutzer bearbeiten" onclose={() => (editing = null)}>
    <form class="stack" onsubmit={saveEdit} novalidate>
      <label><span class="lbl">Name</span><input bind:value={editing.name} /></label>
      <label><span class="lbl">E-Mail</span><input type="email" bind:value={editing.email} /></label>
      <label><span class="lbl">Rolle</span>
        <select bind:value={editing.role} disabled={editing.id === session.user.id}><option value="user">Benutzer</option><option value="admin">Administrator</option></select>
      </label>
      <div>
        <span class="lbl">Projekte ({editProjects.length})</span>
        {#if editProjects.length}
          <ul class="projlist">
            {#each editProjects as p}
              <li><a href={href(`/p/${p.projectId}/members`)} onclick={() => (editing = null)}>{p.name}</a> <span class="badge">{labels.role[p.role]}</span></li>
            {/each}
          </ul>
        {:else}
          <p class="hint">Noch keinem Projekt zugeordnet.</p>
        {/if}
      </div>
      {#if formError}<div class="error-box" role="alert">{formError}</div>{/if}
      <div class="row"><span class="spacer"></span>
        <button type="button" onclick={() => (editing = null)}>Abbrechen</button>
        <button class="primary" disabled={busy}>Speichern</button>
      </div>
    </form>
  </Modal>
{/if}

{#if resetTarget}
  <Modal title="Passwort zurücksetzen" onclose={() => (resetTarget = null)}>
    <div class="stack">
      {#if resetResult}
        <Credentials {...resetResult} note="Alle bisherigen Anmeldungen wurden beendet. Das Startpasswort wird nur jetzt angezeigt." />
        <div class="row"><span class="spacer"></span><button class="primary" onclick={() => (resetTarget = null)}>Fertig</button></div>
      {:else}
        <p>Für <strong>{resetTarget.name}</strong> wird ein neues Startpasswort erzeugt. Alle bestehenden Anmeldungen der Person werden beendet.</p>
        <div class="row"><span class="spacer"></span>
          <button onclick={() => (resetTarget = null)}>Abbrechen</button>
          <button class="primary" onclick={doReset} disabled={busy}>Zurücksetzen</button>
        </div>
      {/if}
    </div>
  </Modal>
{/if}

{#if deleteTarget}
  <Modal title="Benutzer löschen" onclose={() => (deleteTarget = null)}>
    <div class="stack">
      <p><strong>{deleteTarget.name}</strong> ({deleteTarget.email}) wird endgültig gelöscht und aus {deleteTarget.projects} Projekt(en) entfernt.</p>
      <p class="hint">Projekte, deren einzige Eigentümerin bzw. einziger Eigentümer die Person ist, werden an Sie übergeben. Uploads, Befunde und Bewertungen bleiben erhalten.
        Wenn die Person nur vorübergehend keinen Zugriff haben soll, ist <em>Sperren</em> die bessere Wahl.</p>
      <div class="row"><span class="spacer"></span>
        <button onclick={() => (deleteTarget = null)}>Abbrechen</button>
        <button class="danger" onclick={doDelete} disabled={busy}>Endgültig löschen</button>
      </div>
    </div>
  </Modal>
{/if}

<style>
  .signup { margin-bottom: 16px; align-items: center; gap: 20px; }
  .switch { gap: 8px; white-space: nowrap; font-weight: 600; }
  .bar { margin-bottom: 12px; }
  .search { max-width: 280px; }
  .tags { gap: 6px; margin-top: 2px; }
  tr.off td { color: var(--muted); background: var(--surface-2); }
  .actions { display: flex; gap: 6px; justify-content: flex-end; }
  fieldset { border: none; padding: 0; margin: 0; }
  .opt { gap: 8px; }
  .projlist { margin: 6px 0 0; padding-left: 18px; line-height: 1.8; }
</style>
