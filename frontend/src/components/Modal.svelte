<script>
  import { onMount } from 'svelte';
  let { title, onclose, wide = false, children, footer } = $props();
  let box = $state();
  const FOCUSABLE = 'a[href], button:not([disabled]), input:not([disabled]):not([type=hidden]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

  // move focus into the dialog, give it back to the opener on close (R-32)
  onMount(() => {
    const opener = document.activeElement;
    const first = [...box.querySelectorAll(FOCUSABLE)].find((el) => !el.closest('header')) || box.querySelector(FOCUSABLE);
    first?.focus();
    return () => { if (opener instanceof HTMLElement && document.contains(opener)) opener.focus(); };
  });

  function key(e) {
    if (e.key === 'Escape') { e.preventDefault(); onclose?.(); return; }
    if (e.key !== 'Tab' || !box) return;
    const els = [...box.querySelectorAll(FOCUSABLE)].filter((el) => el.offsetParent !== null);
    if (!els.length) return;
    const [first, last] = [els[0], els[els.length - 1]];
    if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus(); }
    else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus(); }
  }
</script>

<svelte:window onkeydown={key} />

<div class="backdrop" role="presentation" onclick={(e) => e.target === e.currentTarget && onclose?.()}>
  <div class="modal card" class:wide role="dialog" aria-modal="true" aria-label={title} bind:this={box}>
    <header class="row">
      <h2 class="grow">{title}</h2>
      <button class="ghost close" aria-label="Dialog schließen" onclick={() => onclose?.()}><span aria-hidden="true">✕</span></button>
    </header>
    <div class="body">{@render children?.()}</div>
    {#if footer}<footer class="row">{@render footer()}</footer>{/if}
  </div>
</div>

<style>
  .backdrop { position: fixed; inset: 0; background: rgba(21, 24, 28, .5); display: grid; place-items: center; z-index: 50; padding: 20px; }
  .modal { box-shadow: var(--e-3); border-top: 4px solid var(--ink); width: min(560px, 100%); max-height: calc(100vh - 40px); display: flex; flex-direction: column; }
  .modal.wide { width: min(1000px, 100%); }
  header { padding: var(--s-3) var(--s-5); border-bottom: 1px solid var(--line); }
  .body { padding: var(--s-5); overflow: auto; }
  footer { padding: var(--s-3) var(--s-5); border-top: 1px solid var(--line); justify-content: flex-end; }
</style>
