<script>
  import { onMount } from 'svelte';
  import { api, fmt } from '../lib/api.js';
  import { session, toast, toastError } from '../lib/state.svelte.js';
  import { route, href } from '../lib/router.svelte.js';

  const tab = $derived(route.parts[1] || 'models');
  let settings = $state(null);
  let users = $state([]);
  let status = $state(null);
  let tests = $state({});

  onMount(async () => {
    try {
      [settings, users, status] = await Promise.all([api.get('/admin/settings/models'), api.get('/admin/users'), api.get('/admin/status')]);
      settings.vision.labels ??= [];
    } catch (e) { toastError(e); }
  });

  async function save() {
    try {
      settings = await api.put('/admin/settings/models', settings);
      toast('Modell-Einstellungen gespeichert');
    } catch (e) { toastError(e); }
  }

  async function test(kind) {
    tests[kind] = { running: true };
    try { tests[kind] = await api.post('/admin/settings/models/test', { kind, settings }); } catch (e) { tests[kind] = { ok: false, error: e.message }; }
  }

  async function updateUser(u, patch) {
    try {
      const upd = await api.patch(`/admin/users/${u.id}`, patch);
      Object.assign(u, upd);
    } catch (e) { toastError(e); }
  }

  const presets = {
    ollama: { baseUrl: 'http://ollama:11434/v1', hint: 'Ollama (OpenAI-kompatibel)' },
    vllm: { baseUrl: 'http://vllm:8000/v1', hint: 'vLLM' },
    lmstudio: { baseUrl: 'http://host.docker.internal:1234/v1', hint: 'LM Studio' },
    openai: { baseUrl: 'https://api.openai.com/v1', hint: 'OpenAI' }
  };
</script>

{#snippet endpointForm(ep, kind, title, desc, extra)}
  <section class="card card-pad stack">
    <div class="row">
      <div class="grow"><h2 class="h3">{title}</h2><p class="hint">{desc}</p></div>
      <label class="row check"><input type="checkbox" bind:checked={ep.enabled} /> aktiv</label>
    </div>
    <div class="grid3">
      <label class="span2"><span class="lbl">Basis-URL (OpenAI-kompatibel)</span><input class="mono" bind:value={ep.baseUrl} placeholder="http://ollama:11434/v1" /></label>
      <label><span class="lbl">Vorlage</span>
        <select onchange={(e) => { if (e.currentTarget.value) ep.baseUrl = presets[e.currentTarget.value].baseUrl; e.currentTarget.value = ''; }}>
          <option value="">– wählen –</option>
          {#each Object.entries(presets) as [k, p]}<option value={k}>{p.hint}</option>{/each}
        </select>
      </label>
      <label><span class="lbl">Modell</span><input class="mono" bind:value={ep.model} placeholder="qwen3:8b" /></label>
      <label><span class="lbl">API-Key</span><input type="password" bind:value={ep.apiKey} placeholder="optional" autocomplete="off" /></label>
      <label><span class="lbl">Timeout (s)</span><input type="number" bind:value={ep.timeoutSec} /></label>
      {#if extra}
        <label><span class="lbl">Temperatur</span><input type="number" step="0.05" bind:value={ep.temperature} /></label>
        <label><span class="lbl">Max. Tokens</span><input type="number" bind:value={ep.maxTokens} /></label>
        <label class="row check"><input type="checkbox" bind:checked={ep.jsonMode} /> JSON-Modus (response_format)</label>
      {/if}
    </div>
    <div class="row">
      <button onclick={() => test(kind)}>Verbindung testen</button>
      {#if tests[kind]?.running}<span class="muted">teste …</span>
      {:else if tests[kind]}
        <span class={tests[kind].ok ? 'ok-box' : 'error-box'}>{tests[kind].ok ? `OK (${tests[kind].ms} ms)${tests[kind].dimensions ? ' · ' + tests[kind].dimensions + ' Dimensionen' : ''}` : tests[kind].error}</span>
      {/if}
    </div>
  </section>
{/snippet}

<div class="page">
  <h1>Administration</h1>
  <nav class="tabs" style="margin-top:14px" aria-label="Administrationsbereiche">
    <a href={href('/admin/models')} class:active={tab === 'models'}>Modelle</a>
    <a href={href('/admin/users')} class:active={tab === 'users'}>Benutzer</a>
    <a href={href('/admin/status')} class:active={tab === 'status'}>System</a>
  </nav>

  {#if tab === 'models' && settings}
    <div class="stack">
      <p class="muted">Alle Modell-Schnittstellen sind OpenAI-kompatibel und unabhängig konfigurierbar – lokal (Ollama, vLLM, LM Studio, llama.cpp) oder extern. Ohne Modelle laufen alle deterministischen Prüfungen trotzdem.</p>
      {@render endpointForm(settings.chat, 'chat', 'Chat-Modell (KI-Review & Fragen)', 'Prüft Datenmodule inhaltlich und beantwortet Fragen. Hier kann auch ein feingetuntes Modell eingetragen werden.', true)}
      {@render endpointForm(settings.embedding, 'embedding', 'Embedding-Modell (Wissensbasis)', 'Optional: semantische Suche und Kontext für den KI-Review, z. B. nomic-embed-text, bge-m3.', false)}

      <section class="card card-pad stack">
        <h2 class="h3">Bildanalyse (Körperteile)</h2>
        <div class="grid3">
          <label><span class="lbl">Anbieter</span>
            <select bind:value={settings.vision.provider}>
              <option value="none">Deaktiviert (nur manuelle Prüfung)</option>
              <option value="heuristic">Integrierte Heuristik (Hautfarbe, nur Fotos)</option>
              <option value="openai">Vision-Sprachmodell (OpenAI-kompatibel)</option>
              <option value="http">Eigener Detektor-Dienst (HTTP)</option>
            </select>
          </label>
          <label><span class="lbl">Min. Konfidenz</span><input type="number" step="0.05" bind:value={settings.vision.minScore} /></label>
          <label><span class="lbl">Max. Kantenlänge an Modell (px)</span><input type="number" bind:value={settings.vision.maxSide} /></label>
        </div>
        {#if settings.vision.provider === 'openai'}
          <div class="grid3">
            <label class="span2"><span class="lbl">Basis-URL</span><input class="mono" bind:value={settings.vision.endpoint.baseUrl} placeholder="http://ollama:11434/v1" /></label>
            <label><span class="lbl">Modell</span><input class="mono" bind:value={settings.vision.endpoint.model} placeholder="qwen2.5vl:7b" /></label>
            <label><span class="lbl">API-Key</span><input type="password" bind:value={settings.vision.endpoint.apiKey} autocomplete="off" /></label>
            <label><span class="lbl">Koordinatenformat der Antwort</span>
              <select bind:value={settings.vision.coordFormat}>
                <option value="norm1000">0–1000 normiert (Qwen3-VL, InternVL)</option>
                <option value="pixel">Pixel (Qwen2.5-VL)</option>
                <option value="norm1">0–1 normiert</option>
                <option value="gemini">0–1000, [ymin,xmin,ymax,xmax] (Gemini/PaliGemma)</option>
              </select>
            </label>
            <label><span class="lbl">Timeout (s)</span><input type="number" bind:value={settings.vision.endpoint.timeoutSec} /></label>
          </div>
          <p class="hint">Grounding-fähige Vision-Modelle erkennen auch gezeichnete Hände in Strichzeichnungen – deutlich robuster als reine Posen-Modelle auf technischen Illustrationen.</p>
        {:else if settings.vision.provider === 'http'}
          <div class="grid3">
            <label class="span2"><span class="lbl">Detektor-URL</span><input class="mono" bind:value={settings.vision.httpUrl} placeholder="http://detector:8000/detect" /></label>
            <label><span class="lbl">API-Key</span><input type="password" bind:value={settings.vision.httpApiKey} autocomplete="off" /></label>
          </div>
          <p class="hint">Vertrag: <code>POST multipart/form-data</code> mit Feld <code>image</code> (PNG) und <code>labels</code>; Antwort <code>{'{"detections":[{"label":"hand","box":[x1,y1,x2,y2],"score":0.9}]}'}</code> in Pixeln. So lassen sich YOLO-, GroundingDINO- oder Sapiens-Modelle anbinden.</p>
        {/if}
        <div class="row">
          <button onclick={() => test('vision')}>Verbindung testen</button>
          {#if tests.vision?.running}<span class="muted">teste …</span>
          {:else if tests.vision}<span class={tests.vision.ok ? 'ok-box' : 'error-box'}>{tests.vision.ok ? `OK (${tests.vision.ms} ms)` : tests.vision.error}</span>{/if}
        </div>
      </section>

      <div class="row savebar"><span class="spacer"></span><button class="primary" onclick={save}>Einstellungen speichern</button></div>
    </div>
  {:else if tab === 'users'}
    <section class="card">
      <table>
        <thead><tr><th>Name</th><th>E-Mail</th><th>Rolle</th><th>Status</th><th>Letzte Anmeldung</th></tr></thead>
        <tbody>
          {#each users as u (u.id)}
            <tr>
              <td><strong>{u.name}</strong></td>
              <td>{u.email}</td>
              <td>
                <select value={u.role} aria-label={`Rolle von ${u.name}`} disabled={u.id === session.user.id} onchange={(e) => updateUser(u, { role: e.currentTarget.value })} style="width:auto">
                  <option value="user">Benutzer</option><option value="admin">Administrator</option>
                </select>
              </td>
              <td>
                <label class="row check"><input type="checkbox" aria-label={`${u.name} aktiv`} checked={!u.disabled} disabled={u.id === session.user.id} onchange={() => updateUser(u, { disabled: !u.disabled })} /> aktiv</label>
              </td>
              <td class="muted">{fmt.date(u.lastLoginAt)}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </section>
  {:else if tab === 'status' && status}
    <div class="grid3">
      <div class="card card-pad"><span class="muted">Benutzer</span><h1>{status.users}</h1></div>
      <div class="card card-pad"><span class="muted">Projekte</span><h1>{status.projects}</h1></div>
      <div class="card card-pad"><span class="muted">Uploads</span><h1>{status.snapshots}</h1></div>
      <div class="card card-pad"><span class="muted">Laufende Analysen</span><h1>{status.activeRuns}</h1></div>
      <div class="card card-pad"><span class="muted">Jobs in Warteschlange (Redis)</span><h1>{status.queueLength}</h1></div>
    </div>
  {/if}
</div>

<style>
  .span2 { grid-column: span 2; }
  .check { gap: 6px; }
  .savebar { position: sticky; bottom: 0; background: var(--bg); padding: 10px 0; }
</style>
