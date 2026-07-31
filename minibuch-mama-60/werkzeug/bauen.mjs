#!/usr/bin/env node
// Baut aus den Markdown-Dateien in inhalt/ eine einzelne HTML-Datei (bau/minibuch.html).
// Danach macht bauen.sh daraus per Chromium ein A5-PDF.
//
// Bewusst ohne npm-Abhaengigkeiten: der unterstuetzte Markdown-Umfang steht in der README.

import { readFileSync, readdirSync, writeFileSync, mkdirSync, existsSync } from "node:fs";
import { join, dirname, basename } from "node:path";
import { fileURLToPath } from "node:url";

const wurzel = join(dirname(fileURLToPath(import.meta.url)), "..");
const inhaltDir = join(wurzel, "inhalt");
const bauDir = join(wurzel, "bau");

const buch = JSON.parse(readFileSync(join(wurzel, "buch.json"), "utf8"));

// ---------------------------------------------------------------- Markdown

const escapeHtml = (s) =>
  s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");

function inline(text) {
  let s = escapeHtml(text);
  // Bild:  ![Bildunterschrift](bilder/foto.jpg)
  s = s.replace(
    /!\[([^\]]*)\]\(([^)]+)\)/g,
    (_, alt, src) =>
      `<img src="${src}" alt="${alt}">${alt ? `<span class="bildunterschrift">${alt}</span>` : ""}`
  );
  // {{Luecke}} = noch zu schreiben, wird im Entwurf sichtbar markiert
  s = s.replace(/\{\{([^}]*)\}\}/g, '<span class="luecke">$1</span>');
  s = s.replace(/\*\*([^*]+)\*\*/g, "<strong>$1</strong>");
  s = s.replace(/(^|[^*])\*([^*]+)\*/g, "$1<em>$2</em>");
  // Maskierter Punkt: "4\. September" bleibt Text und wird keine Liste
  s = s.replace(/\\\./g, ".");
  return s;
}

function markdownZuHtml(md) {
  const zeilen = md
    .replace(/<!--[\s\S]*?-->/g, "") // Schreibhilfen erscheinen nie im Buch
    .split("\n");

  const aus = [];
  let liste = null; // "ul" | "ol" | null
  let zitat = [];

  const listeSchliessen = () => {
    if (liste) {
      aus.push(`</${liste}>`);
      liste = null;
    }
  };
  const zitatSchliessen = () => {
    if (zitat.length) {
      aus.push(`<blockquote>${zitat.map(inline).join("<br>")}</blockquote>`);
      zitat = [];
    }
  };
  const absatzSchliessen = () => {
    listeSchliessen();
    zitatSchliessen();
  };

  let absatz = [];
  const absatzLeeren = () => {
    if (absatz.length) {
      aus.push(`<p>${inline(absatz.join(" "))}</p>`);
      absatz = [];
    }
  };

  for (const roh of zeilen) {
    const z = roh.trim();

    if (z === "") {
      absatzLeeren();
      absatzSchliessen();
      continue;
    }

    if (z === "---") {
      absatzLeeren();
      absatzSchliessen();
      aus.push('<div class="seitenumbruch"></div>');
      continue;
    }

    if (z === "~~~") {
      absatzLeeren();
      absatzSchliessen();
      aus.push('<hr class="zierlinie">');
      continue;
    }

    const ueberschrift = z.match(/^(#{1,3})\s+(.*)$/);
    if (ueberschrift) {
      absatzLeeren();
      absatzSchliessen();
      const stufe = ueberschrift[1].length;
      aus.push(`<h${stufe}>${inline(ueberschrift[2])}</h${stufe}>`);
      continue;
    }

    if (z.startsWith("> ")) {
      absatzLeeren();
      listeSchliessen();
      zitat.push(z.slice(2));
      continue;
    }

    const ul = z.match(/^[-*]\s+(.*)$/);
    if (ul) {
      absatzLeeren();
      zitatSchliessen();
      if (liste !== "ul") {
        listeSchliessen();
        aus.push("<ul>");
        liste = "ul";
      }
      aus.push(`<li>${inline(ul[1])}</li>`);
      continue;
    }

    // "4\. September 2026" ist ein Datum, keine Liste — der maskierte
    // Punkt verhindert, dass daraus ein Aufzählungspunkt wird.
    const ol = /^\d+\\\./.test(z) ? null : z.match(/^\d+\.\s+(.*)$/);
    if (ol) {
      absatzLeeren();
      zitatSchliessen();
      if (liste !== "ol") {
        listeSchliessen();
        aus.push("<ol>");
        liste = "ol";
      }
      aus.push(`<li>${inline(ol[1])}</li>`);
      continue;
    }

    if (/^!\[[^\]]*\]\([^)]+\)$/.test(z)) {
      absatzLeeren();
      absatzSchliessen();
      // Beginnt die Bildunterschrift mit "handschrift" oder "unterschrift",
      // wird das als Layoutklasse verwendet und nicht mitgedruckt.
      const sonderformat = z.match(/^!\[(handschrift|unterschrift)\s*([^\]]*)\]\(([^)]+)\)$/);
      if (sonderformat) {
        const [, klasse, text, quelle] = sonderformat;
        aus.push(
          `<figure class="${klasse}"><img src="${quelle}" alt="${escapeHtml(text)}">` +
            (text ? `<span class="bildunterschrift">${inline(text)}</span>` : "") +
            `</figure>`
        );
      } else {
        aus.push(`<figure>${inline(z)}</figure>`);
      }
      continue;
    }

    // Rohes HTML durchreichen (für Notizseiten und Sonderlayouts)
    if (z.startsWith("<") && z.endsWith(">")) {
      absatzLeeren();
      absatzSchliessen();
      aus.push(z);
      continue;
    }

    absatzSchliessen();
    absatz.push(z);
  }

  absatzLeeren();
  absatzSchliessen();
  return aus.join("\n");
}

// ---------------------------------------------------------------- Dateien

function artVon(dateiname) {
  if (dateiname.includes("titelseite")) return "titelseite";
  if (dateiname.includes("widmung")) return "widmung";
  if (dateiname.includes("teil-")) return "teilseite";
  if (dateiname.includes("notizseiten")) return "notizseiten";
  return "kapitel";
}

const dateien = readdirSync(inhaltDir)
  .filter((d) => d.endsWith(".md"))
  .sort();

if (dateien.length === 0) {
  console.error("Keine .md-Dateien in inhalt/ gefunden.");
  process.exit(1);
}

const abschnitte = dateien.map((d) => {
  const md = readFileSync(join(inhaltDir, d), "utf8");
  const titel = (md.replace(/<!--[\s\S]*?-->/g, "").match(/^#\s+(.*)$/m) || [, ""])[1];
  return { datei: d, art: artVon(basename(d)), titel, html: markdownZuHtml(md) };
});

// ------------------------------------------------ Inhaltsverzeichnis bauen

const verzeichnisEintraege = abschnitte
  .filter((a) => (a.art === "kapitel" || a.art === "teilseite") && a.titel)
  .map(
    (a) =>
      `<li class="${a.art === "teilseite" ? "vz-teil" : "vz-kapitel"}">${escapeHtml(a.titel)}</li>`
  )
  .join("\n");

const verzeichnisHtml = `
<section class="seite inhaltsverzeichnis">
  <h1>Inhalt</h1>
  <ul class="vz">
${verzeichnisEintraege}
  </ul>
</section>`;

// ---------------------------------------------------------------- Zusammenbau

const ersterKapitelIndex = abschnitte.findIndex((a) => a.art === "teilseite" || a.art === "kapitel");

const beta = !!process.env.BETA;

const koerper = abschnitte
  .map((a, i) => {
    let inhalt = a.html;
    if (beta && a.art === "titelseite") {
      const heute = new Date().toLocaleDateString("de-DE", {
        day: "2-digit",
        month: "2-digit",
        year: "numeric",
      });
      inhalt += `\n<p class="testdruck-hinweis">Testdruck vom ${heute} — nicht die endgültige Fassung.<br>Gelb markierte Stellen sind noch offen.</p>`;
    }
    const sektion = `<section class="seite ${a.art}" data-datei="${a.datei}">\n${inhalt}\n</section>`;
    return i === ersterKapitelIndex ? verzeichnisHtml + "\n" + sektion : sektion;
  })
  .join("\n\n");

const css = readFileSync(join(wurzel, "werkzeug", "stil.css"), "utf8");

const html = `<!doctype html>
<html lang="de">
<head>
<meta charset="utf-8">
<title>${escapeHtml(buch.titel)} — ${escapeHtml(buch.untertitel)}</title>
<style>
${css}
</style>
</head>
<body>
${koerper}
</body>
</html>
`;

const ausgabeName = beta ? "minibuch-beta" : "minibuch";
if (!existsSync(bauDir)) mkdirSync(bauDir, { recursive: true });
writeFileSync(join(bauDir, `${ausgabeName}.html`), html, "utf8");

// ---------------------------------------------------------------- Fortschritt

let offen = 0;
const proDatei = [];
for (const a of abschnitte) {
  const md = readFileSync(join(inhaltDir, a.datei), "utf8").replace(/<!--[\s\S]*?-->/g, "");
  const treffer = md.match(/\{\{[^}]*\}\}/g) || [];
  offen += treffer.length;
  if (treffer.length) proDatei.push(`   ${a.datei.padEnd(34)} ${treffer.length}`);
}

const woerter = abschnitte
  .map((a) => a.html.replace(/<[^>]+>/g, " "))
  .join(" ")
  .split(/\s+/)
  .filter((w) => w.length > 1 && !w.startsWith("{{")).length;

console.log(`\n  bau/${ausgabeName}.html geschrieben${beta ? "  (Testdruck-Fassung)" : ""}`);
console.log(`  ${abschnitte.length} Abschnitte, rund ${woerter} Wörter geschrieben`);
if (offen) {
  console.log(`\n  Noch ${offen} offene Lücken {{...}}:`);
  console.log(proDatei.join("\n"));
} else {
  console.log(`\n  Keine offenen Lücken mehr. Das Buch ist fertig.`);
}
console.log("");
