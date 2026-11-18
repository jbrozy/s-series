<script>
  import { onMount, tick } from 'svelte';
  import Modal from './Modal.svelte';

  let { snapshotId, file, line = 0, onclose } = $props();
  let lines = $state(null);
  let error = $state('');
  let box = $state();

  onMount(async () => {
    try {
      const res = await fetch(`/api/snapshots/${snapshotId}/files/${file.id}/content`, { credentials: 'same-origin' });
      if (!res.ok) throw new Error('Datei konnte nicht geladen werden');
      lines = (await res.text()).split('\n');
      await tick();
      box?.querySelector('.hl')?.scrollIntoView({ block: 'center' });
    } catch (e) {
      error = e.message;
    }
  });
</script>

<Modal title={file.dmc || file.path} wide {onclose}>
  {#if error}
    <div class="error-box">{error}</div>
  {:else if !lines}
    <p class="muted">Lade …</p>
  {:else}
    <div class="row meta">
      <span class="mono muted grow ellipsis">{file.path}</span>
      <a class="btn small" href={`/api/snapshots/${snapshotId}/files/${file.id}/content?download=1`}>Herunterladen</a>
    </div>
    <div class="code" bind:this={box}>
      {#each lines as l, i}
        <div class="ln" class:hl={i + 1 === line}><span class="no">{i + 1}</span><span class="tx">{l}</span></div>
      {/each}
    </div>
  {/if}
</Modal>

<style>
  .meta { margin-bottom: 10px; }
  .code { font-family: var(--mono); font-size: 12.5px; background: #0f1722; color: #d6dee8; border-radius: 8px; max-height: 65vh; overflow: auto; padding: 8px 0; }
  .ln { display: flex; white-space: pre; }
  .ln.hl { background: rgba(255, 196, 0, .22); }
  .no { width: 54px; flex: none; text-align: right; padding-right: 12px; color: #6b7a8c; user-select: none; }
  .tx { padding-right: 16px; }
</style>
