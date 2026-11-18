<script>
  import { api } from '../lib/api.js';
  import { toastError } from '../lib/state.svelte.js';

  let { snapshot, canEdit } = $props();
  let count = $state(1000);
  let format = $state('chat');
  let input = $state('text');
  let cleanRatio = $state(0.25);
  let seed = $state(42);
  let focus = $state('semantic');
  let stats = $state(null);
  let busy = $state(false);

  const query = $derived(new URLSearchParams({ count, format, input, cleanRatio, seed, focus }).toString());

  async function preview() {
    busy = true;
    try { stats = await api.get(`/snapshots/${snapshot.id}/synthetic?${query}&stats=1`); } catch (e) { toastError(e); } finally { busy = false; }
  }

  const mutationLabel = {
    clean: 'Fehlerfreie Beispiele',
    step_order: 'Falsche Schrittreihenfolge', missing_warning: 'Fehlender Sicherheitshinweis', value_conflict: 'Widersprüchlicher Wert',
    unit_error: 'Falsche Einheit', meaning_inverted: 'Sinnverkehrte Aussage', procedure_mismatch: 'Schritt passt nicht zum DM',
    duplicate_list_item: 'Doppelter Listeneintrag', duplicate_requirement: 'Doppelte Vorbedingung',
    duplicate_sibling_title: 'Doppelte Überschrift', duplicate_step: 'Doppelter Arbeitsschritt', title_typo: 'Tippfehler im Titel',
    text_typo: 'Tippfehler im Text', title_punctuation: 'Satzzeichen im Titel', forbidden_term: 'Unzulässiger Begriff'
  };
</script>

<div class="layout">
  <section class="card card-pad stack">
    <h2 class="h3">Synthetische Trainingsdaten erzeugen</h2>
    <p class="muted">
      Aus den (überwiegend korrekten) Datenmodulen dieses Uploads werden Fragmente gezogen und gezielt mit Fehlern versehen, die keine Regel findet:
      vertauschte Arbeitsschritte, fehlende Warnhinweise, widersprüchliche Werte, falsche Einheiten, sinnverkehrte Aussagen und Schritte,
      die nicht zum Datenmodul passen. Da der Generator weiß, was er verändert hat, ist das Label sicher. Format wie im KI-Review.
    </p>
    {#if !canEdit}
      <div class="error-box">Nur Redakteure und Eigentümer können Trainingsdaten erzeugen.</div>
    {:else}
      <div class="grid3">
        <label><span class="lbl">Fehlerarten</span>
          <select bind:value={focus}>
            <option value="semantic">Inhaltlich, nicht per Regel prüfbar (empfohlen)</option>
            <option value="all">Inhaltlich + strukturell</option>
            <option value="structural">Nur strukturell (Regel-Nachbildung)</option>
          </select>
        </label>
        <label><span class="lbl">Anzahl Beispiele</span><input type="number" min="10" max="20000" bind:value={count} /></label>
        <label><span class="lbl">Format</span>
          <select bind:value={format}>
            <option value="chat">Chat (messages, für OpenAI/Unsloth/Axolotl)</option>
            <option value="alpaca">Alpaca (instruction/input/output)</option>
          </select>
        </label>
        <label><span class="lbl">Eingabeform</span>
          <select bind:value={input}>
            <option value="text">Gerenderter Text mit Zeilenmarken (wie KI-Review)</option>
            <option value="xml">Rohes XML-Fragment</option>
          </select>
        </label>
        <label><span class="lbl">Anteil fehlerfreier Beispiele</span><input type="number" step="0.05" min="0" max="0.9" bind:value={cleanRatio} /></label>
        <label><span class="lbl">Seed (reproduzierbar)</span><input type="number" bind:value={seed} /></label>
      </div>
      <div class="row">
        <button onclick={preview} disabled={busy}>Statistik berechnen</button>
        <a class="btn primary" href={`/api/snapshots/${snapshot.id}/synthetic?${query}`}>JSONL herunterladen</a>
      </div>
      {#if stats}
        <div class="stats">
          <strong>{stats.written} Beispiele</strong>
          <ul>
            {#each Object.entries(stats.mutations).sort((a, b) => b[1] - a[1]) as [k, v]}
              <li><span>{mutationLabel[k] || k}</span><span>{v}</span></li>
            {/each}
          </ul>
        </div>
      {/if}
    {/if}
  </section>

  <section class="card card-pad stack howto">
    <h2 class="h3">So nutzen Sie die Daten</h2>
    <ol>
      <li>Mehrere korrekte Uploads (verschiedene Projekte) als Datenbasis verwenden. Je vielfältiger, desto besser.</li>
      <li>Vorher testen, ob ein fertiges Modell mit dem KI-Review schon reicht. Finetuning lohnt vor allem für kleine, lokal betriebene Modelle.</li>
      <li>Ein kleines Modell (z. B. Qwen 2.5/3 mit 3 bis 8 Mrd. Parametern) per LoRA auf die JSONL-Datei feintunen (Unsloth, Axolotl, LLaMA-Factory).</li>
      <li>Das Modell z. B. mit Ollama oder vLLM bereitstellen und unter <em>Administration → Modelle</em> als Chat-Modell eintragen.</li>
      <li>Der KI-Review nutzt exakt dasselbe Prompt- und Antwortformat wie die Trainingsdaten.</li>
    </ol>
    <p class="hint">Projektregeln (unzulässige Begriffe, Satzlänge, Schreibweise) fließen in Prompt und Labels ein. Echte Fehler sind subtiler als
      synthetische: Bestätigte und verworfene Befunde aus S-Check eignen sich als Testdatensatz, um ein trainiertes Modell zu bewerten.</p>
  </section>
</div>

<style>
  .layout { display: grid; grid-template-columns: minmax(0, 1.4fr) minmax(0, 1fr); gap: 16px; align-items: start; }
  .stats { background: var(--surface-2); border: 1px solid var(--line); border-radius: var(--r-1); padding: 12px 14px; }
  .stats ul { list-style: none; margin: 8px 0 0; padding: 0; columns: 2; }
  .stats li { display: flex; justify-content: space-between; gap: 12px; padding: 2px 0; break-inside: avoid; }
  .howto ol { margin: 0; padding-left: 20px; line-height: 1.7; }
  @media (max-width: 1000px) { .layout { grid-template-columns: minmax(0, 1fr); } }
</style>
