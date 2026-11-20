<script>
  // Side navigation: general areas and, inside a project, its sections.
  // Desktop: always visible unless hidden in settings. Narrow screens: drawer.
  import { route, href } from '../lib/router.svelte.js';
  import { session } from '../lib/state.svelte.js';
  import { projectNav, projectSections, ui } from '../lib/nav.svelte.js';
  import { fmt } from '../lib/api.js';

  let { drawer = false } = $props();

  const inProject = $derived(route.parts[0] === 'p' && projectNav.project && projectNav.project.id === route.parts[1]);
  const section = $derived(route.parts[2] || 'overview');
  const canEdit = $derived(projectNav.project && ['owner', 'editor'].includes(projectNav.project.myRole));
  const query = $derived(projectNav.snapshotId ? { snap: projectNav.snapshotId } : undefined);
  const releaseText = { approved: 'freigegeben', rejected: 'abgelehnt' };

  function close() { if (drawer) ui.navOpen = false; }
</script>

<nav class="sidebar" aria-label="Hauptnavigation">
  <div class="group">
    <h2 class="gtitle">Arbeitsbereich</h2>
    <ul>
      <li><a href={href('/')} onclick={close} aria-current={route.parts.length === 0 ? 'page' : undefined}>Projekte</a></li>
      <li><a href={href('/einstellungen')} onclick={close} aria-current={route.parts[0] === 'einstellungen' ? 'page' : undefined}>Einstellungen</a></li>
      <li><a href={href('/hilfe')} onclick={close} aria-current={route.parts[0] === 'hilfe' ? 'page' : undefined}>Handbuch</a></li>
      {#if session.user?.role === 'admin'}
        <li><a href={href('/admin')} onclick={close} aria-current={route.parts[0] === 'admin' ? 'page' : undefined}>Administration</a></li>
      {/if}
    </ul>
  </div>

  {#if inProject}
    <div class="group">
      <h2 class="gtitle ptitle" title={projectNav.project.name}>{projectNav.project.name}</h2>
      <ul>
        {#each projectSections as [key, label]}
          {#if key !== 'audit' || canEdit}
            <li>
              <a href={href(`/p/${projectNav.project.id}/${key}`, query)} onclick={close} aria-current={section === key ? 'page' : undefined}>
                <span class="grow">{label}</span>
                {#if key === 'findings' && projectNav.counts.findings != null}
                  <span class="count" aria-label="{projectNav.counts.findings} offen">{fmt.num(projectNav.counts.findings)}</span>
                {/if}
                {#if key === 'images' && projectNav.counts.images}
                  <span class="count warn" aria-label="{projectNav.counts.images} zu prüfen">{projectNav.counts.images}</span>
                {/if}
                {#if key === 'release' && projectNav.release}
                  <span class="state {projectNav.release}">{releaseText[projectNav.release]}</span>
                {/if}
              </a>
            </li>
          {/if}
        {/each}
      </ul>
    </div>
  {/if}
</nav>

<style>
  .sidebar { display: flex; flex-direction: column; gap: var(--s-4); padding: var(--s-4) var(--s-2); }
  .gtitle { font-size: var(--t-xs); font-weight: 600; color: var(--muted); padding: 0 var(--s-3); margin-bottom: var(--s-1); }
  .ptitle { font-size: var(--t-sm); color: var(--ink); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  ul { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 1px; }
  a {
    display: flex; align-items: center; gap: var(--s-2); min-height: 34px; padding: 0 var(--s-3);
    border-radius: var(--r-1); color: var(--ink-2); text-decoration: none; border-left: 3px solid transparent;
  }
  a:hover { background: var(--surface-hover); color: var(--ink); }
  /* current page: tint + weight + bar, not colour alone */
  a[aria-current="page"] { background: var(--accent-soft); color: var(--accent); font-weight: 600; border-left-color: var(--accent); }
  .count.warn { background: var(--warning-soft); color: var(--warning); }
  .state { font-size: var(--t-xs); font-weight: 600; padding: 0 6px; border-radius: var(--r-1); line-height: 18px; }
  .state.approved { background: var(--ok-soft); color: var(--ok); }
  .state.rejected { background: var(--error-soft); color: var(--error); }
  @media (max-width: 1000px) { a { min-height: 44px; } }
</style>
