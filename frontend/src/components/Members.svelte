<script>
  import SortTh from './SortTh.svelte';
  import { createSorter } from '../lib/sort.svelte.js';
  import LoadError from './LoadError.svelte';
  import { onMount } from 'svelte';
  import { api, fmt, labels } from '../lib/api.js';
  import { go } from '../lib/router.svelte.js';
  import { session, toast, toastError } from '../lib/state.svelte.js';
  import Modal from './Modal.svelte';
  import Credentials from './Credentials.svelte';

  let { project } = $props();
  const roleRank = { owner: 0, editor: 1, viewer: 2 };
  const sorter = createSorter('name', { name: (m) => m.name, role: (m) => roleRank[m.role], last: (m) => (m.lastLoginAt ? -new Date(m.lastLoginAt).getTime() : null) }, 'asc', 'scheck.sort.members');
  let members = $state([]);
  const isOwner = $derived(project.myRole === 'owner');
  const isAdmin = $derived(session.user.role === 'admin');
  const owners = $derived(members.filter((m) => m.role === 'owner').length);

  // person picker
  let query = $state('');
  let results = $state([]);
  let picked = $state([]);
  let role = $state('editor');
  let searching = $state(false);
  let open = $state(false);
  let active = $state(-1);
  let timer;
  let signupAllowed = $state(true);

  // new account (admins)
  let newName = $state('');
  let created = $state(null);
  let busy = $state(false);
  let removeTarget = $state(null);

  const looksLikeEmail = $derived(/^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(query.trim()));
  const offerCreate = $derived(isAdmin && looksLikeEmail && !searching && results.length === 0 && !picked.some((p) => p.email === query.trim().toLowerCase()));

  let loadError = $state(null);
  async function loadMembers() {
    loadError = null;
    try { members = await api.get(`/projects/${project.id}/members`); } catch (e) { loadError = e; }
  }
  onMount(async () => {
    await loadMembers();
    signupAllowed = (await api.get('/auth/config').catch(() => ({ allowSignup: true }))).allowSignup;
  });

  function onInput() {
    clearTimeout(timer);
    open = true;
    active = -1;
    timer = setTimeout(search, 200);
  }

  async function search() {
    searching = true;
    try {
      const res = await api.get(`/users?project=${project.id}&q=${encodeURIComponent(query.trim())}`);
      results = res.filter((r) => !picked.some((p) => p.id === r.id));
    } catch (e) {
      results = [];
    } finally {
      searching = false;
    }
  }

  function pick(u) {
    picked = [...picked, u];
    query = '';
    results = [];
    open = false;
    document.getElementById('person-search')?.focus();
  }

  function unpick(u) {
    picked = picked.filter((p) => p.id !== u.id);
  }

  function onKey(e) {
    if (!open || !results.length) return;
    if (e.key === 'ArrowDown') { e.preventDefault(); active = (active + 1) % results.length; }
    else if (e.key === 'ArrowUp') { e.preventDefault(); active = (active - 1 + results.length) % results.length; }
    else if (e.key === 'Enter' && active >= 0) { e.preventDefault(); pick(results[active]); }
    else if (e.key === 'Escape') { open = false; }
  }

  async function addPicked() {
    if (!picked.length) return;
    busy = true;
    try {
      const res = await api.post(`/projects/${project.id}/members`, { userIds: picked.map((p) => p.id), role });
      members = res.members;
      toast(`${res.added} ${res.added === 1 ? 'Person' : 'Personen'} als ${labels.role[role]} hinzugefügt`);
      picked = [];
    } catch (e) { toastError(e); } finally { busy = false; }
  }

  async function createAndAdd(e) {
    e.preventDefault();
    if (!newName.trim()) return toastError(new Error('Bitte einen Namen angeben'));
    busy = true;
    try {
      const res = await api.post(`/projects/${project.id}/members`, { email: query.trim(), role, create: { name: newName } });
      members = res.members;
      created = { name: res.created.name, email: res.created.email, password: res.created.tempPassword };
      query = '';
      newName = '';
      open = false;
    } catch (err) { toastError(err); } finally { busy = false; }
  }

  async function changeRole(m, r) {
    try {
      members = await api.patch(`/projects/${project.id}/members/${m.userId}`, { role: r });
      toast(`${m.name} ist jetzt ${labels.role[r]}`);
    } catch (e) {
      toastError(e);
      members = await api.get(`/projects/${project.id}/members`).catch(() => members);
    }
  }

  async function remove() {
    const m = removeTarget;
    removeTarget = null;
    try {
      const res = await api.del(`/projects/${project.id}/members/${m.userId}`);
      if (m.userId === session.user.id && !isAdmin) {
        toast('Sie haben das Projekt verlassen');
        go('/');
        return;
      }
      members = res;
      toast(`${m.name} entfernt`);
    } catch (e) { toastError(e); }
  }
</script>

<div class="layout">
  {#if loadError}
  <LoadError what="Die Mitglieder" error={loadError} onretry={loadMembers} />
  {:else}
  <section class="card">
    <table>
      <thead><tr><SortTh {sorter} key="name">Name</SortTh><SortTh {sorter} key="role">Rolle</SortTh><SortTh {sorter} key="last">Letzte Anmeldung</SortTh><th><span class="sr-only">Aktionen</span></th></tr></thead>
      <tbody>
        {#each sorter.apply(members) as m (m.userId)}
          <tr class:off={m.disabled}>
            <td>
              <strong>{m.name}</strong>{#if m.userId === session.user.id} <span class="hint">(Sie)</span>{/if}
              {#if m.disabled}<span class="badge">gesperrt</span>{/if}
              <div class="hint">{m.email}</div>
            </td>
            <td>
              {#if isOwner}
                <select value={m.role} aria-label={`Rolle von ${m.name}`} onchange={(e) => changeRole(m, e.currentTarget.value)} style="width:auto"
                  disabled={m.role === 'owner' && owners === 1} title={m.role === 'owner' && owners === 1 ? 'Einziger Eigentümer: zuerst einen weiteren Eigentümer bestimmen' : ''}>
                  {#each Object.entries(labels.role) as [k, v]}<option value={k}>{v}</option>{/each}
                </select>
              {:else}
                <span class="badge">{labels.role[m.role]}</span>
              {/if}
            </td>
            <td class="muted nowrap">{m.lastLoginAt ? fmt.date(m.lastLoginAt) : 'noch nie'}</td>
            <td class="right nowrap">
              {#if m.userId === session.user.id}
                {#if !(m.role === 'owner' && owners === 1)}
                  <button class="small" onclick={() => (removeTarget = m)}>Projekt verlassen</button>
                {/if}
              {:else if isOwner && !(m.role === 'owner' && owners === 1)}
                <button class="small danger" onclick={() => (removeTarget = m)}>Entfernen</button>
              {/if}
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  </section>
  {/if}

  <aside class="stack">
    <section class="card card-pad stack">
      <h2 class="h3">Personen hinzufügen</h2>
      {#if !isOwner}
        <p class="muted">Nur Eigentümer des Projekts können Personen hinzufügen.</p>
      {:else}
        {#if created}
          <Credentials {...created} note="Konto angelegt und zum Projekt hinzugefügt. Das Startpasswort wird nur jetzt angezeigt." />
          <button class="small closecred" onclick={() => (created = null)}>Hinweis schließen</button>
        {/if}

        <div class="combo">
          <label for="person-search"><span class="lbl">Name oder E-Mail</span></label>
          <input id="person-search" role="combobox" aria-expanded={open && results.length > 0} aria-controls="person-results" aria-autocomplete="list"
            aria-activedescendant={active >= 0 ? `person-${active}` : undefined}
            bind:value={query} oninput={onInput} onfocus={() => { open = true; if (!results.length) search(); }} onkeydown={onKey}
            onblur={() => setTimeout(() => (open = false), 150)} placeholder="z. B. Meyer oder meyer@firma.de" autocomplete="off" />
          {#if open && results.length}
            <ul id="person-results" class="results card" role="listbox">
              {#each results as u, i (u.id)}
                <li id={`person-${i}`} role="option" aria-selected={i === active} class:active={i === active}
                  onmousedown={(e) => { e.preventDefault(); pick(u); }}>
                  <strong>{u.name}</strong><span class="hint">{u.email}</span>
                </li>
              {/each}
            </ul>
          {/if}
        </div>
        {#if query.trim() && !searching && !results.length}
          {#if offerCreate}
            <form class="stack newperson" onsubmit={createAndAdd}>
              <p class="hint">Zu <strong>{query.trim()}</strong> gibt es noch kein Konto. Als Administrator können Sie es direkt anlegen:</p>
              <label><span class="lbl">Name der Person</span><input bind:value={newName} autocomplete="off" /></label>
              <button class="primary" disabled={busy}>Konto anlegen und als {labels.role[role]} hinzufügen</button>
            </form>
          {:else}
            <p class="hint">
              Keine passende Person gefunden (oder bereits Mitglied).
              {#if !isAdmin}
                {signupAllowed ? 'Die Person kann sich selbst registrieren und danach hinzugefügt werden.' : 'Bitte einen Administrator bitten, das Konto anzulegen.'}
              {:else}
                Für ein neues Konto die vollständige E-Mail-Adresse eingeben.
              {/if}
            </p>
          {/if}
        {/if}

        {#if picked.length}
          <ul class="picked" aria-label="Ausgewählte Personen">
            {#each picked as p (p.id)}
              <li class="chip active">{p.name}<button class="ghost x" aria-label={`${p.name} entfernen`} onclick={() => unpick(p)}>✕</button></li>
            {/each}
          </ul>
        {/if}

        <label><span class="lbl">Rolle</span>
          <select bind:value={role}>{#each Object.entries(labels.role) as [k, v]}<option value={k}>{v}</option>{/each}</select>
        </label>
        <button class="primary" onclick={addPicked} disabled={busy || !picked.length}>
          {picked.length > 1 ? `${picked.length} Personen hinzufügen` : 'Hinzufügen'}
        </button>
      {/if}
    </section>
    <section class="card card-pad">
      <ul class="hint roles">
        <li><strong>Leser</strong> sehen Uploads, Befunde, Bilder und den Wissensgraphen.</li>
        <li><strong>Redakteure</strong> laden hoch, starten Analysen, bewerten Befunde, bearbeiten Bilder und Regeln.</li>
        <li><strong>Eigentümer</strong> verwalten zusätzlich Mitglieder und das Projekt.</li>
      </ul>
    </section>
  </aside>
</div>

{#if removeTarget}
  <Modal title={removeTarget.userId === session.user.id ? 'Projekt verlassen' : 'Mitglied entfernen'} onclose={() => (removeTarget = null)}>
    <div class="stack">
      <p>
        {#if removeTarget.userId === session.user.id}
          Sie verlieren den Zugriff auf „{project.name}“. Ein Eigentümer kann Sie wieder hinzufügen.
        {:else}
          <strong>{removeTarget.name}</strong> verliert den Zugriff auf „{project.name}“. Das Konto selbst bleibt bestehen.
        {/if}
      </p>
      <div class="row"><span class="spacer"></span>
        <button onclick={() => (removeTarget = null)}>Abbrechen</button>
        <button class="danger" onclick={remove}>{removeTarget.userId === session.user.id ? 'Verlassen' : 'Entfernen'}</button>
      </div>
    </div>
  </Modal>
{/if}

<style>
  .layout { display: grid; grid-template-columns: minmax(0, 1fr) 360px; gap: 16px; align-items: start; }
  tr.off td { color: var(--muted); }
  .combo { position: relative; }
  .results { position: absolute; left: 0; right: 0; top: calc(100% + 4px); z-index: 10; list-style: none; margin: 0; padding: 4px; max-height: 260px; overflow: auto; }
  .results li { padding: 7px 9px; border-radius: var(--r-1); cursor: pointer; display: flex; flex-direction: column; }
  .results li:hover, .results li.active { background: var(--accent-soft); }
  .picked { list-style: none; margin: 0; padding: 0; display: flex; flex-wrap: wrap; gap: 6px; }
  .picked .x { padding: 0 2px; font-size: 12px; }
  .newperson { background: var(--surface-2); border: 1px solid var(--line); border-radius: var(--r-1); padding: 10px 12px; }
  .closecred { align-self: flex-start; }
  .roles { padding-left: 18px; margin: 0; line-height: 1.6; }
  @media (max-width: 1000px) { .layout { grid-template-columns: minmax(0, 1fr); } }
</style>
