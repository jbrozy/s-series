<script>
  let { title, onclose, wide = false, children, footer } = $props();
  function key(e) { if (e.key === 'Escape') onclose?.(); }
</script>

<svelte:window onkeydown={key} />

<div class="backdrop" role="presentation" onclick={(e) => e.target === e.currentTarget && onclose?.()}>
  <div class="modal card" class:wide role="dialog" aria-modal="true" aria-label={title}>
    <header class="row">
      <h2 class="grow">{title}</h2>
      <button class="ghost" aria-label="Schließen" onclick={() => onclose?.()}>✕</button>
    </header>
    <div class="body">{@render children?.()}</div>
    {#if footer}<footer class="row">{@render footer()}</footer>{/if}
  </div>
</div>

<style>
  .backdrop { position: fixed; inset: 0; background: rgba(15, 23, 35, .45); display: grid; place-items: center; z-index: 50; padding: 20px; }
  .modal { width: min(560px, 100%); max-height: calc(100vh - 40px); display: flex; flex-direction: column; }
  .modal.wide { width: min(1000px, 100%); }
  header { padding: 14px 18px; border-bottom: 1px solid var(--line); }
  .body { padding: 18px; overflow: auto; }
  footer { padding: 12px 18px; border-top: 1px solid var(--line); justify-content: flex-end; }
</style>
