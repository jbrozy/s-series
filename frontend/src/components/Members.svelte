<script>
  import { onMount } from 'svelte';
  import { api, fmt, labels } from '../lib/api.js';
  import { session, toast, toastError } from '../lib/state.svelte.js';

  let { project } = $props();
  let members = $state([]);
  let email = $state('');
  let role = $state('editor');
  const isOwner = $derived(project.myRole === 'owner');

  onMount(async () => { members = await api.get(`/projects/${project.id}/members`).catch(() => []); });

  async function add(e) {
    e.preventDefault();
    try {
      members = await api.post(`/projects/${project.id}/members`, { email, role });
      email = '';
      toast('Mitglied hinzugefügt');
    } catch (err) { toastError(err); }
  }
  async function changeRole(m, r) {
    try { members = await api.post(`/projects/${project.id}/members`, { email: m.email, role: r }); } catch (err) { toastError(err); }
  }
  async function remove(m) {
    if (!confirm(`${m.name} aus dem Projekt entfernen?`)) return;
    try { members = await api.del(`/projects/${project.id}/members/${m.userId}`); } catch (err) { toastError(err); }
  }
</script>

<div class="layout">
  <section class="card">
    <table>
      <thead><tr><th>Name</th><th>E-Mail</th><th>Rolle</th><th>Seit</th><th><span class="sr-only">Aktionen</span></th></tr></thead>
      <tbody>
        {#each members as m (m.userId)}
          <tr>
            <td><strong>{m.name}</strong>{#if m.userId === session.user.id} <span class="hint">(Sie)</span>{/if}</td>
            <td>{m.email}</td>
            <td>
              {#if isOwner}
                <select value={m.role} aria-label={`Rolle von ${m.name}`} onchange={(e) => changeRole(m, e.currentTarget.value)} style="width:auto">
                  {#each Object.entries(labels.role) as [k, v]}<option value={k}>{v}</option>{/each}
                </select>
              {:else}{labels.role[m.role]}{/if}
            </td>
            <td class="muted">{fmt.date(m.addedAt)}</td>
            <td class="right">{#if isOwner}<button class="small danger" onclick={() => remove(m)}>Entfernen</button>{/if}</td>
          </tr>
        {/each}
      </tbody>
    </table>
  </section>
  <aside class="card card-pad stack">
    <h2 class="h3">Mitglied hinzufügen</h2>
    {#if isOwner}
      <form class="stack" onsubmit={add}>
        <label><span class="lbl">E-Mail eines registrierten Benutzers</span><input type="email" bind:value={email} required /></label>
        <label><span class="lbl">Rolle</span>
          <select bind:value={role}>{#each Object.entries(labels.role) as [k, v]}<option value={k}>{v}</option>{/each}</select>
        </label>
        <button class="primary">Hinzufügen</button>
      </form>
    {:else}
      <p class="muted">Nur Eigentümer können Mitglieder verwalten.</p>
    {/if}
    <ul class="hint roles">
      <li><strong>Leser</strong> sehen Uploads, Befunde und Bilder.</li>
      <li><strong>Redakteure</strong> laden hoch, starten Analysen, bearbeiten Befunde, Bilder und Regeln.</li>
      <li><strong>Eigentümer</strong> verwalten zusätzlich Mitglieder und das Projekt.</li>
    </ul>
  </aside>
</div>

<style>
  .layout { display: grid; grid-template-columns: 1fr 340px; gap: 16px; align-items: start; }
  .roles { padding-left: 18px; margin: 0; line-height: 1.6; }
</style>
