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
</script>

<svelte:window onclick={() => (menuOpen = false)} />

{#if !session.ready}
  <div class="boot">S-Check wird geladen …</div>
{:else if !session.user}
  <Login />
{:else}
  <a class="skip-link" href="#inhalt" onclick={(e) => { e.preventDefault(); document.getElementById('inhalt')?.focus(); }}>Zum Inhalt springen</a>
  <header class="top">
    <a class="brand" href={href('/')}>
      <svg viewBox="0 0 32 32" width="26" height="26" aria-hidden="true"><rect width="32" height="32" rx="7" fill="#1f6fb8"/><path d="M9 17.5l4.5 4.5L23 11" fill="none" stroke="#fff" stroke-width="3.2" stroke-linecap="round" stroke-linejoin="round"/></svg>
      <span>S-Check</span>
      <small>S-Series Inhaltsprüfung</small>
    </a>
    <nav aria-label="Hauptnavigation">
      <a href={href('/')} class:active={route.parts.length === 0 || route.parts[0] === 'p'}>Projekte</a>
      {#if session.user.role === 'admin'}
        <a href={href('/admin')} class:active={route.parts[0] === 'admin'}>Administration</a>
      {/if}
    </nav>
    <div class="spacer"></div>
    <div class="user">
      <button class="ghost userbtn" aria-haspopup="menu" aria-expanded={menuOpen} onclick={(e) => { e.stopPropagation(); menuOpen = !menuOpen; }}>
        <span class="avatar">{session.user.name.slice(0, 1).toUpperCase()}</span>
        <span>{session.user.name}</span>
      </button>
      {#if menuOpen}
        <div class="menu card">
          <div class="menu-head">
            <strong>{session.user.name}</strong>
            <span class="muted">{session.user.email}</span>
          </div>
          <a href={href('/account')}>Konto & Passwort</a>
          <button class="ghost" onclick={logout}>Abmelden</button>
        </div>
      {/if}
    </div>
  </header>

  <main id="inhalt" tabindex="-1">
    {#if route.parts[0] === 'p' && route.parts[2] === 'images' && route.parts[3]}
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

<div class="toasts" role="status" aria-live="polite">
  {#each toasts.items as t (t.id)}
    <div class="toast {t.kind}">{t.message}</div>
  {/each}
</div>

<style>
  main:focus { outline: none; }
  .boot { padding: 80px; text-align: center; color: var(--muted); }
  .top {
    position: sticky; top: 0; z-index: 20;
    display: flex; align-items: center; gap: 28px;
    height: 54px; padding: 0 22px;
    background: var(--header); color: #fff;
  }
  .brand { display: flex; align-items: center; gap: 10px; color: #fff; font-weight: 700; font-size: 16px; }
  .brand:hover { text-decoration: none; }
  .brand small { font-weight: 400; color: #9fb0c3; font-size: 12.5px; margin-left: 2px; }
  nav { display: flex; gap: 4px; }
  nav a { color: #c3cfdc; padding: 6px 12px; border-radius: 6px; font-weight: 500; }
  nav a:hover { color: #fff; background: rgba(255,255,255,.08); text-decoration: none; }
  nav a.active { color: #fff; background: rgba(255,255,255,.12); }
  .spacer { flex: 1; }
  .user { position: relative; }
  .userbtn { color: #e6edf5; }
  .userbtn:hover { background: rgba(255,255,255,.1) !important; }
  .avatar {
    width: 26px; height: 26px; border-radius: 50%; background: #2c4a6b; color: #fff;
    display: inline-grid; place-items: center; font-weight: 700; font-size: 13px;
  }
  .menu { position: absolute; right: 0; top: 44px; min-width: 230px; padding: 6px; display: flex; flex-direction: column; color: var(--ink); }
  .menu-head { padding: 8px 10px 10px; border-bottom: 1px solid var(--line); margin-bottom: 4px; display: flex; flex-direction: column; }
  .menu a, .menu button { padding: 8px 10px; border-radius: 6px; color: var(--ink); text-align: left; justify-content: flex-start; }
  .menu a:hover { background: var(--surface-2); text-decoration: none; }
  .toasts { position: fixed; right: 20px; bottom: 20px; display: flex; flex-direction: column; gap: 8px; z-index: 100; }
  .toast { padding: 10px 14px; border-radius: 8px; background: #1d2939; color: #fff; box-shadow: var(--shadow); max-width: 420px; font-size: 13.5px; }
  .toast.error { background: var(--error); }
  .toast.ok { background: #1f5130; }
</style>
