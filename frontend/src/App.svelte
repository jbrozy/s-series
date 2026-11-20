<script>
  import { onMount, tick } from 'svelte';
  import { api } from './lib/api.js';
  import { route, go, href } from './lib/router.svelte.js';
  import { session, toasts } from './lib/state.svelte.js';
  import { prefs, loadPrefs, applyPrefs, savePrefs } from './lib/prefs.svelte.js';
  import { projectNav, ui } from './lib/nav.svelte.js';
  import Login from './routes/Login.svelte';
  import Projects from './routes/Projects.svelte';
  import Project from './routes/Project.svelte';
  import ImageEditor from './routes/ImageEditor.svelte';
  import Admin from './routes/Admin.svelte';
  import Account from './routes/Account.svelte';
  import Report from './routes/Report.svelte';
  import Settings from './routes/Settings.svelte';
  import Handbook from './routes/Handbook.svelte';
  import Sidebar from './components/Sidebar.svelte';
  import CommandPalette from './components/CommandPalette.svelte';
  import HelpPanel from './components/HelpPanel.svelte';
  import ShortcutsDialog from './components/ShortcutsDialog.svelte';
  import { desktop, loadDesktop, serverHost } from './lib/desktop.svelte.js';

  applyPrefs();

  let loadedFor = '';
  let startHandled = false;
  onMount(async () => {
    await loadDesktop();
    try {
      session.user = desktop.on && !desktop.server ? null : await api.get('/auth/me');
    } catch {
      session.user = null;
    }
    session.ready = true;
  });

  // settings arrive with the user record (also right after login)
  $effect(() => {
    const u = session.user;
    if (u && u.id !== loadedFor) {
      loadedFor = u.id;
      loadPrefs(u.preferences);
    }
  });

  $effect(() => {
    if (!session.ready) return;
    if (!session.user && route.parts[0] !== 'login') go('/login');
    if (session.user && (route.parts[0] === 'login' || (!startHandled && route.parts.length === 0))) {
      startHandled = true;
      if (prefs.startPage === 'last' && prefs.lastProject) go(`/p/${prefs.lastProject}/overview`);
      else if (route.parts[0] === 'login') go('/');
    }
    if (session.user) startHandled = true;
  });

  // remember the last opened project for the start page setting
  $effect(() => {
    const pid = route.parts[0] === 'p' ? route.parts[1] : '';
    if (pid && session.user && pid !== prefs.lastProject && projectNav.project?.id === pid) savePrefs({ lastProject: pid }).catch(() => {});
  });

  // close the mobile navigation on page change
  $effect(() => { void route.path; ui.navOpen = false; });

  async function logout(query) {
    menuOpen = false;
    if (query?.server) desktop.wantServer = true;
    await api.post('/auth/logout').catch(() => {});
    session.user = null;
    loadedFor = '';
    go('/login', query);
  }

  let menuOpen = $state(false);
  let userBtn = $state();
  let navBtn = $state();
  let drawer = $state();

  const wideQuery = matchMedia('(min-width: 1001px)');
  let isWide = $state(wideQuery.matches);
  wideQuery.addEventListener?.('change', (e) => { isWide = e.matches; if (isWide) ui.navOpen = false; });
  function toggleNav() {
    if (isWide) savePrefs({ navHidden: !prefs.navHidden }).catch(() => {});
    else ui.navOpen = !ui.navOpen;
  }
  $effect(() => {
    if (ui.navOpen) tick().then(() => drawer?.querySelector('a')?.focus());
  });
  const navVisible = $derived(!prefs.navHidden);
  function trapDrawer(e) {
    if (e.key !== 'Tab') return;
    const links = [...drawer.querySelectorAll('a[href], button')];
    if (!links.length) return;
    const [first, last] = [links[0], links[links.length - 1]];
    if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus(); }
    else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus(); }
  }

  // ---- keyboard: Strg+K, F1, Esc always; single keys only if enabled (WCAG 2.1.4)
  let pendingG = false;
  let gTimer;
  function typing(t) {
    return t instanceof HTMLElement && (t.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(t.tagName));
  }
  const gTargets = { p: '/', e: '/einstellungen' };
  const gSections = { u: 'overview', b: 'findings', f: 'release', r: 'rules', i: 'images', m: 'members', w: 'knowledge', t: 'training', l: 'audit' };
  function onKey(e) {
    if (!session.user || session.user.mustChangePassword) return;
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'k') { e.preventDefault(); ui.paletteOpen = true; return; }
    if (e.key === 'F1') { e.preventDefault(); ui.helpOpen = !ui.helpOpen; return; }
    if (e.key === 'Escape') {
      if (menuOpen) { menuOpen = false; userBtn?.focus(); }
      else if (ui.navOpen) { ui.navOpen = false; navBtn?.focus(); }
      return;
    }
    if (!prefs.shortcuts || e.ctrlKey || e.metaKey || e.altKey || typing(e.target) || ui.paletteOpen || document.querySelector('[role=dialog][aria-modal=true]')) return;
    if (pendingG) {
      pendingG = false;
      clearTimeout(gTimer);
      const k = e.key.toLowerCase();
      if (gTargets[k]) { e.preventDefault(); go(gTargets[k]); }
      else if (gSections[k] && projectNav.project && route.parts[0] === 'p') {
        e.preventDefault();
        go(`/p/${projectNav.project.id}/${gSections[k]}`, projectNav.snapshotId ? { snap: projectNav.snapshotId } : undefined);
      }
      return;
    }
    if (e.key === '/') { e.preventDefault(); ui.paletteOpen = true; }
    else if (e.key === '?') { e.preventDefault(); ui.shortcutsOpen = true; }
    else if (e.key === 'g') { pendingG = true; gTimer = setTimeout(() => (pendingG = false), 1200); }
  }
  const isMac = typeof navigator !== 'undefined' && /Mac/.test(navigator.platform);
</script>

<svelte:window onclick={() => (menuOpen = false)} onkeydown={onKey} />

{#if !session.ready}
  <div class="boot" role="status">S-Check wird geladen …</div>
{:else if !session.user}
  <Login />
{:else}
  <a class="skip-link no-print" href="#inhalt" onclick={(e) => { e.preventDefault(); document.getElementById('inhalt')?.focus(); }}>Zum Inhalt springen</a>
  <header class="topbar no-print">
    {#if !session.user.mustChangePassword}
      <button class="ghost navbtn" bind:this={navBtn} onclick={toggleNav} aria-controls="sidebar" aria-label="Navigation"
        aria-expanded={isWide ? navVisible : ui.navOpen}>
        <span class="burger" aria-hidden="true"></span><span class="navlabel">Navigation</span>
      </button>
    {/if}
    <a class="brand" href={href('/')}>S-Check</a>
    {#if !session.user.mustChangePassword}
      <button class="searchbtn" onclick={() => (ui.paletteOpen = true)} aria-keyshortcuts="Control+K" aria-label="Suche öffnen">
        <span class="stext">Suchen nach Projekt, DMC, Befund …</span>
        <span class="skeys" aria-hidden="true"><kbd>{isMac ? '⌘' : 'Strg'}</kbd><kbd>K</kbd></span>
      </button>
      <span class="spacer"></span>
      <button class="ghost topbtn" onclick={() => (ui.helpOpen = !ui.helpOpen)} aria-expanded={ui.helpOpen} aria-keyshortcuts="F1">Hilfe</button>
    {:else}
      <span class="spacer"></span>
    {/if}
    <div class="user">
      <button class="ghost userbtn" bind:this={userBtn} aria-label="Benutzermenü: {session.user.name}" aria-controls="usermenu" aria-expanded={menuOpen} onclick={(e) => { e.stopPropagation(); menuOpen = !menuOpen; }}>
        <span class="avatar" aria-hidden="true">{session.user.name.slice(0, 1).toUpperCase()}</span>
        <span class="uname">{session.user.name}</span>
      </button>
      {#if menuOpen}
        <div class="menu" id="usermenu">
          <div class="menu-head">
            <strong>{session.user.name}</strong>
            <span class="muted">{session.user.email}</span>
            {#if desktop.on}<span class="muted">Server: {serverHost()}</span>{/if}
          </div>
          {#if !session.user.mustChangePassword}
            <a href={href('/account')}>Konto und API-Tokens</a>
            <a href={href('/einstellungen')}>Einstellungen</a>
            <button class="ghost" onclick={() => (ui.shortcutsOpen = true)}>Tastenkürzel</button>
          {/if}
          {#if desktop.on && !desktop.locked}
            <button class="ghost" onclick={() => logout({ server: '1' })}>Server wechseln</button>
          {/if}
          <button class="ghost" onclick={() => logout()}>Abmelden</button>
        </div>
      {/if}
    </div>
  </header>

  <div class="shell">
    {#if !session.user.mustChangePassword}
      <div id="sidebar" class="side no-print" class:hidden={!navVisible}>
        <Sidebar />
      </div>
      {#if ui.navOpen}
        <div class="drawer-backdrop no-print" role="presentation" onclick={() => (ui.navOpen = false)}></div>
        <div class="drawer no-print" bind:this={drawer} role="dialog" aria-modal="true" aria-label="Navigation" tabindex="-1" onkeydown={trapDrawer}>
          <Sidebar drawer />
        </div>
      {/if}
    {/if}
    <main id="inhalt" class="shell-main" class:withnav={navVisible && !session.user.mustChangePassword} tabindex="-1">
      {#if session.user.mustChangePassword}
        <Account forced />
      {:else if route.parts[0] === 'p' && route.parts[2] === 'report'}
        {#key route.parts[1] + (route.query.snap || '')}
          <Report pid={route.parts[1]} sid={route.query.snap || ''} />
        {/key}
      {:else if route.parts[0] === 'p' && route.parts[2] === 'images' && route.parts[3]}
        {#key route.parts[3]}
          <ImageEditor pid={route.parts[1]} iid={route.parts[3]} />
        {/key}
      {:else if route.parts[0] === 'p' && route.parts[1]}
        {#key route.parts[1]}
          <Project pid={route.parts[1]} />
        {/key}
      {:else if route.parts[0] === 'admin'}
        <Admin />
      {:else if route.parts[0] === 'account'}
        <Account />
      {:else if route.parts[0] === 'einstellungen'}
        <Settings />
      {:else if route.parts[0] === 'hilfe'}
        <Handbook />
      {:else}
        <Projects />
      {/if}
    </main>
  </div>

  {#if ui.paletteOpen}<CommandPalette />{/if}
  {#if ui.helpOpen && !session.user.mustChangePassword}<HelpPanel />{/if}
  {#if ui.shortcutsOpen}<ShortcutsDialog />{/if}
{/if}

<div class="toasts no-print" role="status" aria-live="polite">
  {#each toasts.items as t (t.id)}
    <div class="toast {t.kind}"><span class="tlabel">{t.kind === 'error' ? 'Fehler:' : t.kind === 'ok' ? 'Erledigt:' : 'Hinweis:'}</span> {t.message}</div>
  {/each}
</div>

<style>
  main:focus { outline: none; }
  .boot { padding: var(--s-8); text-align: center; color: var(--muted); }

  .topbar {
    position: sticky; top: 0; z-index: 30;
    display: flex; align-items: center; gap: var(--s-3);
    height: var(--topbar-h); padding: 0 var(--s-4);
    background: var(--surface); border-bottom: 1px solid var(--line);
  }
  .brand { font-weight: 700; font-size: var(--t-xl); color: var(--ink); text-decoration: none; flex: none; padding: 0 var(--s-2); }
  .brand:hover { color: var(--accent); }
  .navbtn { gap: var(--s-2); padding: 0 var(--s-2); }
  .burger, .burger::before, .burger::after { display: block; width: 16px; height: 2px; background: var(--ink-2); position: relative; }
  .burger::before, .burger::after { content: ''; position: absolute; left: 0; }
  .burger::before { top: -5px; }
  .burger::after { top: 5px; }
  .searchbtn {
    flex: 0 1 420px; justify-content: space-between; font-weight: 400; color: var(--muted);
    background: var(--surface-2); border-color: var(--line-strong);
  }
  .searchbtn:hover:not(:disabled) { color: var(--ink); }
  .skeys { display: inline-flex; gap: 2px; }
  .spacer { flex: 1; }
  .topbtn { color: var(--ink-2); }
  .user { position: relative; flex: none; }
  .userbtn { padding: 0 var(--s-2); gap: var(--s-2); font-weight: 600; color: var(--ink-2); }
  .avatar {
    width: 28px; height: 28px; border-radius: 50%; background: var(--accent-soft); color: var(--accent);
    display: inline-grid; place-items: center; font-weight: 700; font-size: 13px;
  }
  .menu {
    position: absolute; right: 0; top: 42px; min-width: 260px; padding: var(--s-1); z-index: 50;
    display: flex; flex-direction: column; background: var(--surface); border: 1px solid var(--line); border-radius: var(--r-1); box-shadow: var(--e-2);
  }
  .menu-head { padding: var(--s-2) var(--s-3) var(--s-3); border-bottom: 1px solid var(--line); margin-bottom: var(--s-1); display: flex; flex-direction: column; overflow-wrap: anywhere; }
  .menu a, .menu button { padding: var(--s-2) var(--s-3); min-height: 36px; border-radius: var(--r-1); color: var(--ink); text-align: left; justify-content: flex-start; text-decoration: none; font-weight: 400; display: flex; align-items: center; }
  .menu a:hover { background: var(--surface-hover); }

  .shell { display: flex; min-height: calc(100vh - var(--topbar-h)); }
  .side {
    position: sticky; top: var(--topbar-h); align-self: flex-start; flex: none;
    width: var(--sidebar-w); height: calc(100vh - var(--topbar-h)); overflow: auto;
    background: var(--surface-2); border-right: 1px solid var(--line);
  }
  .side.hidden { display: none; }
  .shell-main { flex: 1; min-width: 0; }
  .drawer-backdrop { position: fixed; inset: var(--topbar-h) 0 0 0; background: var(--overlay); z-index: 34; }
  .drawer { position: fixed; top: var(--topbar-h); left: 0; bottom: 0; width: min(300px, 86vw); z-index: 35; overflow: auto;
    background: var(--surface-2); border-right: 1px solid var(--line); box-shadow: var(--e-3); }

  .toasts { position: fixed; right: var(--s-5); bottom: var(--s-5); display: flex; flex-direction: column; gap: var(--s-2); z-index: 100; }
  .toast {
    padding: var(--s-3) var(--s-4); border-radius: var(--r-1); background: var(--surface); color: var(--ink);
    border: 1px solid var(--line); box-shadow: var(--e-2); max-width: 440px; font-size: var(--t-sm);
  }
  .toast .tlabel { font-weight: 700; }
  .toast.ok .tlabel { color: var(--ok); }
  .toast.error { border-color: var(--error); }
  .toast.error .tlabel { color: var(--error); }

  @media (max-width: 1000px) {
    .side { display: none; }
    .navbtn { min-height: 44px; }
  }
  @media (max-width: 760px) {
    .topbar { gap: var(--s-2); padding: 0 var(--s-2); }
    .navlabel, .uname, .stext, .skeys { display: none; }
    .searchbtn { flex: none; padding: 0 var(--s-3); }
    .searchbtn::before { content: 'Suche'; font-size: var(--t-sm); color: var(--ink-2); font-weight: 600; }
    .toasts { left: var(--s-3); right: var(--s-3); bottom: var(--s-3); }
  }
</style>
