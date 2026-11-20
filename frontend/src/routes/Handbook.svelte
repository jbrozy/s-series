<script>
  // In-app handbook: every page, shortcuts, glossary, applicable standards. Works offline.
  import { href } from '../lib/router.svelte.js';
  import { helpPages, shortcuts, glossary, standards } from '../lib/help.js';
  const order = ['projects', 'overview', 'findings', 'release', 'images', 'rules', 'knowledge', 'training', 'members', 'audit', 'report', 'account', 'settings', 'admin'];
</script>

<div class="page">
  <header class="pagehead">
    <div class="titles">
      <nav class="crumbs" aria-label="Pfad"><ol><li><a href={href('/')}>Projekte</a></li><li aria-current="page">Handbuch</li></ol></nav>
      <h1>Handbuch</h1>
      <p class="sub">Was jede Seite tut und wie Sie damit arbeiten. Hilfe zur gerade geöffneten Seite gibt es jederzeit mit <kbd>F1</kbd>.</p>
    </div>
  </header>

  <div class="layout">
    <nav class="toc card" aria-label="Inhalt des Handbuchs">
      <h2 class="tt">Inhalt</h2>
      <ol>
        <li><a href="#/hilfe" onclick={(e) => { e.preventDefault(); document.getElementById('h-ablauf')?.scrollIntoView(); }}>Typischer Ablauf</a></li>
        {#each order as k}<li><a href="#/hilfe" onclick={(e) => { e.preventDefault(); document.getElementById('h-' + k)?.scrollIntoView(); }}>{helpPages[k].title}</a></li>{/each}
        <li><a href="#/hilfe" onclick={(e) => { e.preventDefault(); document.getElementById('h-keys')?.scrollIntoView(); }}>Tastenkürzel</a></li>
        <li><a href="#/hilfe" onclick={(e) => { e.preventDefault(); document.getElementById('h-glossar')?.scrollIntoView(); }}>Begriffe</a></li>
      </ol>
    </nav>

    <div class="stack content">
      <section class="card card-pad stack" id="h-ablauf">
        <h2>Typischer Ablauf</h2>
        <ol class="steps">
          <li>Projekt anlegen und Mitglieder hinzufügen.</li>
          <li>Business Rules festlegen oder aus dem BREX-Datenmodul übernehmen; Freigabekriterien setzen.</li>
          <li>Export hochladen (XML, SGML oder HTML, als ZIP oder Ordner). Er wird eingelesen und geprüft.</li>
          <li>Befunde sichten: zuweisen, korrigieren lassen, bestätigen oder begründet verwerfen.</li>
          <li>Korrigierten Export erneut hochladen. Der Vergleich zeigt, was neu und was weggefallen ist.</li>
          <li>Freigeben oder ablehnen und den Prüfbericht ablegen.</li>
        </ol>
      </section>

      {#each order as k}
        <section class="card card-pad stack" id={'h-' + k}>
          <h2>{helpPages[k].title}</h2>
          <p>{helpPages[k].purpose}</p>
          <ul>{#each helpPages[k].steps as s}<li>{s}</li>{/each}</ul>
        </section>
      {/each}

      <section class="card card-pad stack" id="h-keys">
        <h2>Tastenkürzel</h2>
        <p class="hint">Einzeltasten-Kürzel lassen sich unter <a href={href('/einstellungen')}>Einstellungen</a> abschalten. In Eingabefeldern sind sie inaktiv.</p>
        <table>
          <thead><tr><th>Tasten</th><th>Wirkung</th></tr></thead>
          <tbody>
            {#each shortcuts as s}
              <tr><td class="nowrap">{#each s.keys as key, i}{#if i}<span class="then"> {s.keys[0] === 'Strg' ? '+' : 'dann'} </span>{/if}<kbd>{key}</kbd>{/each}</td><td>{s.text}</td></tr>
            {/each}
          </tbody>
        </table>
      </section>

      <section class="card card-pad stack" id="h-glossar">
        <h2>Begriffe</h2>
        <dl class="gloss">
          {#each glossary as [term, def]}<div><dt>{term}</dt><dd>{def}</dd></div>{/each}
        </dl>
      </section>

      <section class="card card-pad stack">
        <h2>Gestaltung nach Norm</h2>
        <p>Die Oberfläche folgt diesen Vorgaben zur Software-Ergonomie und Barrierefreiheit:</p>
        <ul>{#each standards as s}<li>{s}</li>{/each}</ul>
      </section>
    </div>
  </div>
</div>

<style>
  .layout { display: grid; grid-template-columns: 240px minmax(0, 1fr); gap: var(--s-5); align-items: start; }
  .toc { position: sticky; top: calc(var(--topbar-h) + var(--s-4)); padding: var(--s-3) var(--s-4); }
  .tt { font-size: var(--t-sm); color: var(--muted); margin-bottom: var(--s-2); }
  .toc ol { margin: 0; padding-left: var(--s-4); display: flex; flex-direction: column; gap: var(--s-1); font-size: var(--t-sm); }
  .content section { scroll-margin-top: calc(var(--topbar-h) + var(--s-4)); }
  .content h2 { font-size: var(--t-xl); }
  ul, .steps { margin: 0; padding-left: var(--s-5); display: flex; flex-direction: column; gap: var(--s-2); max-width: 80ch; }
  .then { color: var(--muted); font-size: var(--t-sm); }
  .gloss { margin: 0; display: flex; flex-direction: column; }
  .gloss div { display: grid; grid-template-columns: 200px minmax(0, 1fr); gap: var(--s-4); padding: var(--s-2) 0; border-bottom: 1px solid var(--line); }
  .gloss div:last-child { border-bottom: none; }
  dt { font-weight: 600; }
  dd { margin: 0; }
  @media (max-width: 900px) {
    .layout { grid-template-columns: minmax(0, 1fr); }
    .toc { position: static; }
    .gloss div { grid-template-columns: minmax(0, 1fr); gap: var(--s-1); }
  }
</style>
