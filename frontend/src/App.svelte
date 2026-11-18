<script>
  import { onMount } from 'svelte';
  import { api } from './lib/api.js';
  import { route, go, href } from './lib/router.svelte.js';
  import { session, toasts } from './lib/state.svelte.js';
  import Login from './routes/Login.svelte';
  import Projects from './routes/Projects.svelte';
  import Project from './routes/Project.svelte';
  import ImageEditor from './routes/ImageEditor.svelte';
  import Admin from './routes/Admin.svelte';
  import Account from './routes/Account.svelte';
  import Report from './routes/Report.svelte';

  onMount(async () => {
    try {
      session.user = await api.get('/auth/me');
    } catch {
      session.user = null;
    }
    session.ready = true;
  });

  $effect(() => {
    if (!session.ready) return;
    if (!session.user && route.parts[0] !== 'login') go('/login');
    if (session.user && route.parts[0] === 'login') go('/');
  });

  async function logout() {
    await api.post('/auth/logout').catch(() => {});
    session.user = null;
    go('/login');
  }

  let menuOpen = $state(false);
  let userBtn = $state();
</script>

<svelte:window
  onclick={() => (menuOpen = false)}
  onkeydown={(e) => { if (e.key === 'Escape' && menuOpen) { menuOpen = false; userBtn?.focus(); } }}
/>

{#if !session.ready}
  <div class="boot">S-Check wird geladen …</div>
{:else if !session.user}
  <Login />
{:else}
  <a class="skip-link no-print" href="#inhalt" onclick={(e) => { e.preventDefault(); document.getElementById('inhalt')?.focus(); }}>Zum Inhalt springen</a>
  <header class="top no-print">
    <a class="brand" href={href('/')}>
      <span>S-Check</span>
      <small>S-Series Inhaltsprüfung</small>
    </a>
    {#if !session.user.mustChangePassword}
    <nav aria-label="Hauptnavigation">
      <a href={href('/')} class:active={route.parts.length === 0 || route.parts[0] === 'p'}>Projekte</a>
      {#if session.user.role === 'admin'}
        <a href={href('/admin')} class:active={route.parts[0] === 'admin'}>Administration</a>
      {/if}
    </nav>
    {/if}
    <div class="spacer"></div>
    <div class="user">
      <button class="ghost userbtn" bind:this={userBtn} aria-label="Benutzermenü: {session.user.name}" aria-controls="usermenu" aria-expanded={menuOpen} onclick={(e) => { e.stopPropagation(); menuOpen = !menuOpen; }}>
        <span class="avatar">{session.user.name.slice(0, 1).toUpperCase()}</span>
        <span class="uname">{session.user.name}</span>
      </button>
      {#if menuOpen}
        <div class="menu card" id="usermenu">
          <div class="menu-head">
            <strong>{session.user.name}</strong>
            <span class="muted">{session.user.email}</span>
          </div>
          {#if !session.user.mustChangePassword}<a href={href('/account')}>Konto & Passwort</a>{/if}
          <button class="ghost" onclick={logout}>Abmelden</button>
        </div>
      {/if}
    </div>
  </header>

  <main id="inhalt" tabindex="-1">
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
    {:else}
      <Projects />
    {/if}
  </main>
{/if}

<div class="toasts no-print" role="status" aria-live="polite">
  {#each toasts.items as t (t.id)}
    <div class="toast {t.kind}">{t.message}</div>
  {/each}
</div>

<style>
  main:focus { outline: none; }
  .boot { padding: var(--s-8); text-align: center; color: var(--muted); }
  .top {
    position: sticky; top: 0; z-index: 20;
    display: flex; align-items: center; gap: var(--s-5);
    height: 56px; padding: 0 var(--s-5);
    background: var(--header); color: var(--header-ink);
    border-bottom: 3px solid var(--accent);
  }
  .brand { display: flex; align-items: center; gap: var(--s-3); color: var(--header-ink); flex: none; }
  .brand:hover { text-decoration: none; }
  .brand span { font-family: var(--font-head); font-weight: 600; font-size: 20px; }
  .brand small { font-size: var(--t-sm); color: var(--header-muted); padding-left: var(--s-3); border-left: 1px solid #3a3f46; }
  nav { display: flex; gap: var(--s-1); align-self: stretch; min-width: 0; }
  nav a {
    display: flex; align-items: center; padding: 0 var(--s-3);
    color: var(--header-muted); font-weight: 500;
    border-bottom: 3px solid transparent; margin-bottom: -3px;
  }
  nav a:hover { color: var(--header-ink); text-decoration: none; }
  nav a.active { color: var(--header-ink); border-bottom-color: var(--focus); }
  .spacer { flex: 1; }
  .user { position: relative; flex: none; }
  .userbtn { color: var(--header-ink); padding: var(--s-1) var(--s-2); }
  .userbtn:hover { background: rgba(255, 255, 255, .08) !important; }
  .avatar {
    width: 28px; height: 28px; border-radius: var(--r-1); background: var(--header-ink); color: var(--header);
    display: inline-grid; place-items: center; font-family: var(--font-head); font-weight: 600; font-size: 14px;
  }
  .menu {
    position: absolute; right: 0; top: 46px; min-width: 240px; padding: var(--s-1);
    display: flex; flex-direction: column; color: var(--ink); box-shadow: var(--e-2);
  }
  .menu-head { padding: var(--s-2) var(--s-3) var(--s-3); border-bottom: 1px solid var(--line); margin-bottom: var(--s-1); display: flex; flex-direction: column; overflow-wrap: anywhere; }
  .menu a, .menu button { padding: var(--s-2) var(--s-3); border-radius: var(--r-1); color: var(--ink); text-align: left; justify-content: flex-start; }
  .menu a:hover { background: var(--surface-2); text-decoration: none; }
  .toasts { position: fixed; right: var(--s-5); bottom: var(--s-5); left: auto; display: flex; flex-direction: column; gap: var(--s-2); z-index: 100; }
  .toast {
    padding: var(--s-3) var(--s-4); border-radius: var(--r-1); background: var(--header); color: var(--header-ink);
    box-shadow: var(--e-2); max-width: 420px; font-size: var(--t-sm); border-left: 4px solid var(--accent);
  }
  .toast.error { border-left-color: #e5484d; }
  .toast.ok { border-left-color: #3fa66b; }
  @media (max-width: 900px) { .brand small { display: none; } }
  @media (max-width: 640px) {
    .top { gap: var(--s-3); padding: 0 var(--s-3); }
    .brand span { font-size: 17px; }
    nav a { padding: 0 var(--s-2); }
    .uname { display: none; }
    .toasts { left: var(--s-3); right: var(--s-3); bottom: var(--s-3); }
  }
</style>
