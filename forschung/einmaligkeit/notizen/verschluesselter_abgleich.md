# Verschluesselter Abgleich: Privacy-preserving biometric deduplication (1:N ohne Klartext-Templates), Stand 28.11.2026

Research notes. Recherche-Umgebung: eprint.iacr.org, arxiv.org, mpc.cs.berkeley.edu und die GitHub-API waren aus dieser Sitzung NICHT erreichbar (DNS blockiert). Viele Zahlen stammen daher aus Suchmaschinen-Snippets der Primaerquellen (ePrint/arXiv-Seiten), nicht aus Volltext-Lektuere. Das ist bei jeder Zahl als "(Snippet)" vermerkt, wo relevant. github.com (HTML) war erreichbar.

## 1. World (Worldcoin) SMPC/AMPC fuer Iris-Codes: Architektur, Leistung, Repos, Loeschung

### Takeaway
World betreibt die Iris-Eindeutigkeitspruefung als 3-Parteien-MPC (Secret Sharing, semi-honest, honest majority = hoechstens 1 boese Partei) bei Universitaeten/Nethermind; das Paper (ePrint 2024/705) meldet 4,29 Mrd. Iris-Code-Vergleiche/s mit 8 H100 pro Partei, aber das offene Repo `worldcoin/iris-mpc` hat (Stand Nov. 2026) den GPU-Pfad stillgelegt und nutzt jetzt CPU-Linear-Scan plus CPU-HNSW. Loeschung der zentralen Iris-Code-DB ist eine Eigenaussage von World, die Regulatoren angezweifelt haben.

### Cited Findings
- Paper: "Large-Scale MPC: Scaling Private Iris Code Uniqueness Checks to Millions of Users" (Bloemen, Gillespie, Kales, Sippl, Walch; TACEO Graz u.a.), IACR ePrint 2024/705 / arXiv 2405.04463 (Mai 2024). Durchsatz: **4,29 Mrd. Iris-Code-Vergleiche pro Sekunde im 3-Parteien-MPC, jede Partei mit 8 NVIDIA H100**; die GPU-Implementierung erfuellt laut Autoren die Anforderungen der Worldcoin Foundation und sollte in World ID eingesetzt werden (Snippet) — [ePrint 2024/705](https://eprint.iacr.org/2024/705); [arXiv 2405.04463v2](https://arxiv.org/html/2405.04463v2)
- Repo `worldcoin/iris-mpc` (Rust): "three-party secure multi-party computation iris-matching service"; Lizenz **MIT OR Apache-2.0** (Dual); ca. 53 Stars, 27 Forks, 2.210 Commits auf `main`, 50 offene PRs (Abruf 28.11.2026) — [GitHub iris-mpc](https://github.com/worldcoin/iris-mpc)
- Parteien laut README: Party 0 = "fau" (Uni Erlangen-Nuernberg), Party 1 = "berkeley-university", Party 2 = "kaist-university"; Sicherheitsmodell **"semi-honest, honest majority"**, hoechstens eine adversariale Partei — [GitHub iris-mpc](https://github.com/worldcoin/iris-mpc)
- Architekturwechsel: README sagt "CPU linear scan is the maintained matching service"; es gibt zusaetzlich eine CPU-HNSW-Implementierung (`iris-mpc-hawk`). Der GPU-Pfad ist "retired": CUDA-Kernels, cuBLAS/NCCL, `iris-mpc-gpu`, `cudarc` aus dem Quellbaum entfernt; keine Aenderung an Matching, Wire-Format oder Schema noetig; Rollback ohne Datenmigration moeglich. Ein Grund oder Datum wird nicht genannt — [docs/gpu-retirement.md](https://raw.githubusercontent.com/worldcoin/iris-mpc/main/docs/gpu-retirement.md)
- "Current CPU writers to the anonymous-statistics service use HNSW origins and lifted distances" — d.h. es gibt einen Statistik-Dienst, der aus dem MPC abgeleitete Distanzinformationen erhaelt — [docs/gpu-retirement.md](https://raw.githubusercontent.com/worldcoin/iris-mpc/main/docs/gpu-retirement.md)
- Schluesselrotation in Produktion: alter Entschluesselungsschluessel muss nutzbar bleiben, bis Clients gecachte Schluessel erneuern — [GitHub iris-mpc](https://github.com/worldcoin/iris-mpc)
- AMPC ("Anonymized MPC") angekuendigt Mai 2025: Iris-Code wird anonymisiert, auf das Nutzergeraet gegeben, secret-shared und Ende-zu-Ende-verschluesselt getrennt an jeden Rechenknoten geschickt; Betreiber: Nethermind (speichert die anonymisierte DB), FAU, KAIST, UC Berkeley RDI; **weder World Foundation noch Tools for Humanity betreiben Knoten**; Unterschied zu SMPC: nur ein binaeres Match/Nicht-Match-Ergebnis wird herausgegeben statt Klartext — [Crowdfund Insider, Mai 2025](https://www.crowdfundinsider.com/2025/05/240624-world-id-privacy-and-performance-now-enhanced-with-ampc/); weitere Partnerschaften (Trust Stamp u.a.) — [Biometric Update, Juni 2025](https://www.biometricupdate.com/202506/world-trust-stamp-each-unveil-biometric-mpc-partnerships)
- Personal Custody: Orb schickt Bilder und MPC-Fragmente als Paket ans Nutzergeraet und loescht eigene Kopien; "Tools For Humanity and World Foundation have fully deleted their database of iris codes"; Iris-Code wird in randomisierte Fragmente zerlegt, die bei gleichem Input jedes Mal anders aussehen (Eigenaussage World) — [World Privacy FAQ](https://world.org/blog/world/world-privacy-faqs); [World FAQ](https://world.org/faqs)
- Regulatoren in Brasilien, Hongkong, Philippinen, Thailand u.a. beanstandeten u.a. unzureichende Einwilligung, Daten Minderjaehriger und **nicht nachpruefbare Loeschbehauptungen** (Aggregator) — [Wikipedia: World (blockchain)](https://en.wikipedia.org/wiki/World_(blockchain))
- Verbreitung: >12 Mio. verifizierte Menschen (April 2026, Sekundaerquelle, niedrige Qualitaet) — [webscraft.org](https://webscraft.org/blog/scho-take-world-orb-i-world-id-povniy-rozbir?lang=en)
- "Quantum secure" taucht in Presseberichten zu World-SMPC auf; nicht in technischer Doku verifiziert — [Crowdfund Insider](https://www.crowdfundinsider.com/2025/05/240624-world-id-privacy-and-performance-now-enhanced-with-ampc/)

### Inferences
- Rechenbeispiel (eigene Extrapolation aus 4,29e9 Vergleichen/s): ein 1:N-Abgleich gegen 10 Mio. Codes ≈ 2,3 ms, gegen 1 Mrd. ≈ 0,23 s reine Vergleichszeit — sofern "Vergleich" die Iris-Rotationen bereits einschliesst (im Snippet nicht klar). Das ist mit 24 H100 erkauft.
- Der Umstieg auf CPU-Linear-Scan + HNSW deutet darauf hin, dass Betriebskosten/Betreibbarkeit bei Universitaetsknoten wichtiger wurden als Rohdurchsatz; HNSW im MPC bedeutet allerdings, dass Graph-Struktur/Nachbarschaftsinformation existiert (moegliche Leckage-Flaeche, siehe Abschnitt 6).
- Honest-majority/semi-honest heisst: 2 kolludierende Parteien (von 3) koennen Iris-Codes rekonstruieren; eine einzelne aktiv boesartige Partei kann Korrektheit (nicht notwendig Vertraulichkeit) verletzen.

### Gaps
- Volltext von ePrint 2024/705 nicht abrufbar (Host blockiert): genaue Galeriegroesse im Benchmark, Latenz pro Anfrage, Netzwerkbandbreite, ob Rotationen in den 4,29e9 enthalten sind.
- Kein offizielles World-Engineering-Blog zu AMPC/HNSW gefunden; Datum und Grund der GPU-Stilllegung unbekannt.
- Rolle von Nethermind (Speicher vs. Rechenpartei) im aktuellen README nicht bestaetigt (README nennt nur FAU/Berkeley/KAIST als Parteien).
- Unabhaengige Pruefung der Loeschbehauptung: nicht gefunden.

## 2. FHE-basierte biometrische Suche: HERS, Blind-Match, CryptoFace, BioZKFHE, BioBind; realistische Latenz

### Takeaway
FHE-1:N ist forschungsseitig bis 100 Mio. (HERS, ~500 s–10 min pro Anfrage, Gesichts-Embeddings, reduzierte Dimension) demonstriert; verifizierbare FHE-Varianten (BioZKFHE) liegen bei 22–44 s fuer nur 10k–40k Eintraege, und Ende-zu-Ende-FHE inkl. Merkmalsextraktion (CryptoFace) braucht ~23 min pro Bild auf CPU. Fuer Echtzeit-Dedup bei 10 Mio.+ ist FHE deutlich teurer als MPC.

### Cited Findings
- **HERS** (Engelsma, Jain, Boddeti, MSU): Dimensionsreduktion der Templates, FHE-Verschluesselung, Suche auf verschluesselter Galerie; 100-Mio-Galerie in <10 min bei ~2 % Genauigkeitsverlust; Baseline 38,4 h pro Anfrage → 25 min, Speicher 9 TB → 280 GB; zweistufig (16D-Vorfilter, dann 192D) 4.500 s → 500 s; 2020-Preprint meldete 296 s bei 1 Mio. Galerie; Journal IEEE TBIOM 2022 — [arXiv 2003.12197](https://arxiv.org/abs/2003.12197v2); [MSU Talk](https://hal.cse.msu.edu/talks/hers-encrypted-search-at-scale/); [MSU Paper-Seite](https://hal.cse.msu.edu/papers/hers-encrypted-image-search/)
- **Blind-Match** (Choi, Kim, Song, Woo, Kim; CIKM 2024, DOI 10.1145/3627673.3680017): HE-optimierte Kosinus-Aehnlichkeit mit aufgeteilten Merkmalsvektoren; 99,55 % Rank-1 auf PolyU-Fingerabdruck mit 16-D-Vektor; **6.144 Templates in 0,74 s mit 128-D-Vektor**; Code auf GitHub — [arXiv 2408.06167](https://arxiv.org/abs/2408.06167v2); [dblp](https://dblp.org/rec/conf/cikm/ChoiKSWK24.html)
- **CryptoFace** (Ao, Boddeti, MSU; CVPR 2025, S. 19197–19206; ePrint 2025/1556; arXiv 2509.00332): erstes Ende-zu-Ende-FHE-Gesichtserkennungssystem (Extraktion, Speicherung, Matching verschluesselt) mit Mixture of shallow patch CNNs; **9.845 s → 1.364 s pro Bild auf CPU** (7,2x schneller als FHE-NN-Stand der Technik), bis +8,8 % Verifikationsgenauigkeit; Code offen — [CVF Open Access](https://openaccess.thecvf.com/content/CVPR2025/html/Ao_CryptoFace_End-to-End_Encrypted_Face_Recognition_CVPR_2025_paper.html); [ePrint 2025/1556](https://eprint.iacr.org/2025/1556); [arXiv 2509.00332](https://arxiv.org/html/2509.00332v1)
- **BioZKFHE** (arXiv 2607.22065, Juli 2026): "Scalable Encrypted Biometric Identification via Verifiable Homomorphic Similarity Evaluation"; bis 67 % weniger verschluesselter Speicher; **~22–44 s Ende-zu-Ende inkl. Beweisverifikation bei 10k–40k Eintraegen** (Snippet) — [arXiv 2607.22065](https://arxiv.org/pdf/2607.22065)
- BioZKFHE laut BioBind-Beschreibung: BGV-verschluesselte Galerie in Ciphertext-Bloecken; ein Rechenknoten wertet 1:N aus und haengt pro Block einen oeffentlich pruefbaren Beweis an; ein **Threshold-Komitee entschluesselt nur das Urteil**; Annahme "honest client", arbeitet auf einem Galerie-Snapshot — [arXiv 2611.12636](https://arxiv.org/pdf/2611.12636)
- **BioBind** (arXiv 2611.12636, ca. Anfang Nov. 2026): "Binding Biometric Deduplication to Zero-Knowledge Authentication"; vier Backend-Schnittstellenbedingungen (I1–I4) binden Dedup-Anfrage, Galerie-Einfuegung und ausgegebenes Commitment an **einen** erfassten Vektor; capture-gebundene ZK-Authentisierung verknuepft spaetere Nutzung mit Enrollment-Template und frischer, challenge-gebundener Geraete-Probe; Enrollment-Station macht Erfassung/Extraktion/Ableitung, 1:N-Backend macht private Dedup und authentisierte Galerie-Updates; vergleicht mit BioZKFHE und World-MPC (Snippets) — [arXiv 2611.12636](https://arxiv.org/html/2611.12636)
- BioBind-Ergebnisse: auf Bosphorus-Datenbank **umgehen kooperative Personen mit veraenderter Erscheinung die Dedup in 1,8 % der Re-Enrollment-Versuche** am Betriebsschwellwert; E2E-Lauf mit einem verpassten Duplikat; verschluesseltes Enrollment: drei Historien mit je 16.384 Registrierungen ohne Dekodierfehler an getesteten Checkpoints; Tabellenfragment "Enrollment preparation 2312.12 ms" (Kontext unklar) (Snippets) — [arXiv 2611.12636](https://arxiv.org/pdf/2611.12636)
- Weitere 2025/26-Arbeiten: "Bidirectional Biometric Authentication Using Transciphering and (T)FHE" — [arXiv 2506.12802](https://arxiv.org/html/2506.12802v1); "Biometric Bound Credentials for Age Verification" (Bindung ohne gespeicherte Templates) — [arXiv 2509.07465](https://arxiv.org/html/2509.07465v1)

### Inferences
- Lineare Extrapolation (eigene Rechnung, Einzelanfrage): HERS ~500 s/100 Mio. → ~50 s bei 10 Mio., ~5 s bei 1 Mio., ~80–100 min bei 1 Mrd. Blind-Match 6.144/0,74 s → ~2 min bei 1 Mio., ~20 min bei 10 Mio. (ohne Parallelisierung). BioZKFHE (verifizierbar) ~1 ms/Eintrag → ~3–5 h bei 10 Mio. Diese Zahlen sind Hardware- und Dimensions-abhaengig und nur Groessenordnungen.
- FHE loest "Server sieht keine Templates", aber jemand haelt den Entschluesselungsschluessel; erst Threshold-Entschluesselung (wie BioZKFHE) verteilt das Vertrauen — dann ist die Kollusionsschwelle des Komitees die eigentliche Sicherheitsgrenze, analog zu MPC.
- BioBinds 1,8 %-Umgehungsrate zeigt: Die Kryptografie ist nicht der Engpass fuer "eine Person = ein Konto"; die biometrische FMR/FNMR (insb. Gesicht) begrenzt die Einmaligkeit.

### Gaps
- BioBind-Volltext (Autoren, genaue Latenzen, Kryptobaustein-Wahl, Lizenz) nicht lesbar (arxiv.org blockiert); nur Snippets.
- Keine belastbare Quelle fuer FHE-1:N bei 1 Mrd. gefunden.
- Lizenz der HERS/CryptoFace/Blind-Match-Repos nicht verifiziert.

## 3. Fuzzy Extractors / Secure Sketches / Cancelable Biometrics: Stand, Sicherheitsbits, stabiler Hash-Nullifier?

### Takeaway
Die Iris-Fuzzy-Extractor-Linie (Fuller/Simhadri/Steel u.a.) kam von 32 Bit (ISC 2019, spaeter mit Analysefehler) ueber 65 Bit bei nur 12 % TAR (2024) zu behaupteten 105 Bit bei 92 % TAR (CCS 2025). Das ist fuer 1:1-Schluesselableitung relevant, ersetzt aber 1:N-Deduplikation nicht: ein deterministischer, stabiler Iris-Hash-Nullifier mit hoher TAR und kryptografischer Staerke ist nicht demonstriert, und Helper-Data muss weiterhin gespeichert werden.

### Cited Findings
- "Reusable authentication from the iris" / ePrint 2017/1177 (Simhadri et al.): je nach Version 45 Bit bzw. **32 Bit Sicherheit**, auch bei mehrfacher Schluesselableitung aus derselben Iris; Autoren raeumen ein, 32 Bit seien fuer ein sicheres System unzureichend; ISC 2019 publiziert — [ePrint 2017/1177](https://eprint.iacr.org/2017/1177); [Springer ISC 2019](https://link.springer.com/chapter/10.1007/978-3-030-30215-3_23)
- Spaetere Arbeit berichtet einen **Bug in der ISC-2019-Analyse, der die Staerke reduziert** — [ePrint 2024/100](https://eprint.iacr.org/2024/100.pdf)
- ePrint 2024/100 ("FiveEyes"): ~18 Bit besser als Stand der Technik bei vergleichbarer TAR; **65 Bit Sicherheit (87 Bit mit Passwort) bei nur 12 % TAR** — [ePrint 2024/100](https://eprint.iacr.org/2024/100.pdf)
- "Fuzzy Extractors are Practical: Cryptographic Strength Key Derivation from the Iris" (CCS 2025): **105 Bit Entropie bei 92 % TAR**; relevante Sicherheitsmasszahl ist das Minimum der Min-Entropie ueber Teilmengen (Eigenaussage Abstract) — [Adelphi Scholarly Works](https://scholarlyworks.adelphi.edu/esploro/outputs/conferenceProceeding/Fuzzy-Extractors-are-Practical-Cryptographic-Strength/991004518492306266)
- Panja et al.: robuster wiederverwendbarer Fuzzy Extractor (computational, LWE-basierte Linie), auf Iris-Datensatz implementiert — [ePrint 2023/284](https://eprint.iacr.org/2023/284)
- Reusable Fuzzy Extractors (Grundlagen, Eurocrypt 2016) — [IACR Archive](https://www.iacr.org/archive/eurocrypt2016/96650231/96650231.pdf)
- JPMorganChase-Arbeit "Scalable Secure Biometric Authentication without Auxiliary Identifiers" (2026) nutzt Fuzzy Extractors fuer Authentisierung allein ueber Biometrie (d.h. ohne Benutzer-ID, also ein 1:N-aehnliches Problem) — [arXiv 2604.25071](https://arxiv.org/html/2604.25071v1); [ePrint 2026/825](https://eprint.iacr.org/2026/825)
- Angriffsforschung: "Reconstructing Protected Biometric Templates from Binary Authentication Results" (Jan. 2026) — selbst nur binaere Match-Ergebnisse geschuetzter Templates erlauben Rekonstruktion — [arXiv 2601.17620](https://arxiv.org/abs/2601.17620?context=cs)

### Inferences
- Warum stabile Schluessel schwer sind: Iris-Codes verschiedener Aufnahmen derselben Person unterscheiden sich typischerweise um einen erheblichen Bruchteil der Bits, und die Bits sind korreliert (geringe echte Min-Entropie); hohe Fehlertoleranz frisst Entropie, deshalb der Zielkonflikt TAR vs. Bits (12 % TAR bei 65 Bit vs. 92 % bei 105 Bit in der neuesten, noch nicht unabhaengig geprueften Arbeit).
- Hash-Nullifier fuer Dedup: Ein Fuzzy Extractor braucht oeffentliche Helper-Daten pro Enrollment; fuer Dedup muesste man beim neuen Enrollment ohne Identitaet gegen alle Helper-Daten pruefen (1:N ueber N Helper-Strings) — das ist wieder eine lineare Suche, und jede Helper-Datei leakt etwas Entropie. Ein helper-freier deterministischer Nullifier (gleiche Person → gleicher Hash) bei 92 %+ TAR und <1e-6 FMR ist in den gefundenen Quellen nicht gezeigt. Selbst 8 % FNMR hiesse: ~8 % Mehrfachregistrierungen moeglich.
- Cancelable Biometrics (transformierte Templates) schuetzen vor Wiederverwendung zwischen Systemen, aber nicht vor dem Betreiber, der die Transformation kennt.

### Gaps
- Volltexte (ePrint) nicht lesbar; FAR/FMR-Werte der CCS-2025-Arbeit und Datensatz (vermutlich ND-0405/IITD) nicht bestaetigt.
- Fuzzy Extractors fuer Gesicht (Embeddings): keine aktuelle belastbare Quelle gefunden.
- Keine Quelle gefunden, die einen helper-freien Iris-Nullifier mit kryptografischer Staerke fuer Sybil-Resistenz demonstriert.

## 4. ZK-Beweise ueber Biometrie (BioZero, zkML), On-Device + ZK (Humanity Protocol, Keyless)

### Takeaway
ZK-Ansaetze (BioZero, BioBind, Humanity) loesen vor allem 1:1-Authentisierung gegen ein committetes Template und oeffentliche Pruefbarkeit; die 1:N-Deduplikation selbst laeuft auch dort ueber MPC/FHE-Backends oder ueber nicht nachgewiesene Betreiberaussagen. Humanity Protocols ZK-/Palm-Architektur ist nur in Sekundaerquellen beschrieben, ohne veroeffentlichte Audits.

### Cited Findings
- **BioZero** (arXiv 2409.17509, 26.09.2024; Lai, Wang, Zhang, Yang, Liew — Autorenliste aus Katalog): Pedersen-Commitments + homomorphe Berechnung + ZK; dezentral, durch Smart Contracts effizient ausfuehrbar; Ziel Sybil-Resistenz ohne Biometrie on-chain offenzulegen; spaetere Version umbenannt in "Privacy-Preserving and Publicly Verifiable On-Chain Biometric Authentication via Homomorphic Commitments and Zero-Knowledge Proofs" — [arXiv 2409.17509](https://arxiv.org/abs/2409.17509); [Pith](https://pith.science/paper/2409.17509)
- BioBind ordnet BioZero als "identity-bound, publicly verifiable authentication over committed templates" ein (also 1:1, nicht 1:N-Dedup) — [arXiv 2611.12636](https://arxiv.org/pdf/2611.12636)
- ZK-Series: privacy-preserving Authentisierung mit zeitlichen Biometriedaten — [arXiv 2506.19393](https://arxiv.org/html/2506.19393v1)
- **Humanity Protocol** (Handflaeche): Geraet erzeugt bei Scan einen ZK-"Proof of Trust" zu Credentials; Pipeline "lokale Merkmalsextraktion → verschluesselt → gehasht → IPFS → Human ID" (nur ein Blog); Speicherung eines Hashes statt Biometrie; Verteilung auf "zkProofers"-Knoten; Basis-Chain widerspruechlich (Polygon CDK zkEVM vs. Arbitrum); fehlende veroeffentlichte Sicherheitsaudits — [Messari](https://messari.io/report/understanding-humanity-protocol-a-comprehensive-overview); [Alea Research](https://alearesearch.io/reports/perspectives/humanity); [Mitosis University](https://university.mitosis.org/humanity-protocol-biometric-sybil-resistance-for-decentralized-identity/); [Substack arndxt](https://threadingontheedge.substack.com/p/my-first-proof-of-humanity)

### Inferences
- "Hash der Biometrie" fuer Dedup funktioniert nur, wenn die Biometrie deterministisch quantisiert wird (siehe Abschnitt 3); sonst erkennt der Hash keine Wiederholungs-Enrollments. Behauptungen dieser Art ohne technisches Papier sollten als unbelegt gelten.
- zkML-Matching auf dem Geraet beweist nur, dass ein Modell korrekt auf *irgendeinen* Input lief; ohne vertrauenswuerdige Erfassungs-Hardware (Orb, Enrollment-Station, Attestierung) kann ein Nutzer gefaelschte Inputs einspeisen. BioBind adressiert das ausdruecklich durch "capture-bound" Probes.

### Gaps
- Keyless (Biometrie + MPC, "Zero-Knowledge Biometrics"): keine Quelle gefunden.
- Keine Benchmarks fuer zkML-basiertes 1:N-Matching gefunden.
- Humanity-Protocol-Whitepaper/technische Doku nicht gefunden.

## 5. Frameworks: Reife, Lizenz, Leistung

### Takeaway
Belegt: TFHE-rs (Zama) unter BSD-3-Clause-Clear (keine Patentlizenz!), OpenFHE unter BSD-2-Clause, iris-mpc unter MIT/Apache-2.0. Lizenzen von MP-SPDZ, MOTION, ABY, SEAL, Lattigo, Concrete konnten in dieser Sitzung nicht verifiziert werden (GitHub-API blockiert). Einzige produktiv betriebene Grossskalen-Loesung fuer Iris-Dedup ist World iris-mpc (eigene Rust-Implementierung, nicht auf MP-SPDZ & Co. aufgebaut).

### Cited Findings
- TFHE-rs: Lizenz BSD 3-Clause Clear, Copyright Zama — [tfhe-rs LICENSE](https://github.com/zama-ai/tfhe-rs/blob/main/LICENSE)
- BSD-3-Clause-Clear gewaehrt ausdruecklich **keine Patentlizenz**; kommerzielle Nutzung des Codes erlaubt, Patentrechte aber nicht abgedeckt — [SPDX](https://spdx.org/licenses/BSD-3-Clause-Clear.html); [choosealicense](https://choosealicense.com/licenses/bsd-3-clause-clear/)
- Zama kuendigt in seiner NIST-Threshold-Einreichung an, eine Liste oeffentlich verfuegbarer Patentansprueche zu veroeffentlichen — [NIST Zama Submission](https://csrc.nist.gov/csrc/media/Projects/threshold-cryptography/documents/TCall-1/Zama-PW01.pdf)
- OpenFHE: 2-Klausel-BSD; Community-Projekt mit Duality, Samsung, Intel, MIT u.a. — [OpenFHE Poster ITU 2022](https://www.itu.int/en/ITU-T/Workshops-and-Seminars/2022/0901/Documents/Posters/OpenFHE.pdf); [OpenFHE Paper ePrint 2022/915](https://eprint.iacr.org/2022/915.pdf)
- Lattigo: Go-Bibliothek von EPFL-LDS und Tune Insight mit Multiparty-HE-Erweiterungen (Threshold-Entschluesselung) — [EPFL Infoscience](https://infoscience.epfl.ch/record/305447)
- Microsoft SEAL: unterstuetzt BGV, BFV, CKKS — [arXiv 2312.14434](https://arxiv.org/pdf/2312.14434)
- SoK zu FHE-Compilern/Bibliotheken (Vergleich Reife/Usability) — [Viand et al. 2021 SoK](https://systems.cs.columbia.edu/private-systems-class/papers/Viand2021SoK.pdf)
- Multiparty-FHE-Landschaft (Uebersicht) — [Fenbushi](https://fenbushi.vc/?p=799)
- iris-mpc: MIT OR Apache-2.0, 3 Parteien, CPU-Linear-Scan gewartet — [GitHub iris-mpc](https://github.com/worldcoin/iris-mpc)

### Inferences
- Fuer Hamming-Distanz ueber binaere Iris-Codes ist Secret-Sharing-MPC (additive/replizierte Shares, Vergleiche als Skalarprodukte) um Groessenordnungen schneller als FHE, wie die World-Zahlen (Mrd. Vergleiche/s) vs. HERS (~2e5 Vergleiche/s bei 100 Mio./500 s) zeigen.
- Fuer ein Projekt mit Go-Stack waere Lattigo (Multiparty-CKKS/BFV) der naheliegende FHE-Baustein; iris-mpc (Rust, permissive Lizenz) der naheliegende MPC-Referenzcode.

### Gaps
- Nicht verifiziert (nur Vorwissen, bitte vor Verwendung pruefen): Microsoft SEAL = MIT; Lattigo = Apache-2.0; MP-SPDZ = eigene BSD-artige CSIRO/Data61-Lizenz; MOTION = MIT; ABY = LGPL-3.0; Zama Concrete = BSD-3-Clause-Clear; Sharemind = proprietaer (Cybernetica, akademische Lizenz). Sterne/letzte Releases nicht abrufbar (GitHub-API in Sitzung gesperrt).
- Keine vergleichbaren Benchmarks (Hamming/Skalarprodukt pro Sekunde) fuer MP-SPDZ, MOTION, ABY gefunden.

## 6. Sicherheitsvorbehalte: malicious vs. semi-honest, Kollusion, Distanzleckage, Galerie-Integritaet

### Takeaway
Die produktive World-Loesung ist nur semi-honest mit honest majority (2 von 3 kolludierend = Bruch); FHE verlagert das Vertrauen auf Schluesselinhaber bzw. Threshold-Komitee; selbst binaere Match-Ausgaben koennen Templates leaken; Galerie-Integritaet (wer fuegt was ein) ist ein eigenes Problem, das BioBind/BioZKFHE explizit adressieren.

### Cited Findings
- World iris-mpc: semi-honest, honest majority, hoechstens 1 adversariale Partei — [GitHub iris-mpc](https://github.com/worldcoin/iris-mpc)
- AMPC gibt nur binaere Match-Ergebnisse heraus (frueher SMPC mehr) — [Crowdfund Insider](https://www.crowdfundinsider.com/2025/05/240624-world-id-privacy-and-performance-now-enhanced-with-ampc/); dennoch erhaelt ein anonymer Statistik-Dienst "lifted distances" aus HNSW — [docs/gpu-retirement.md](https://raw.githubusercontent.com/worldcoin/iris-mpc/main/docs/gpu-retirement.md)
- Template-Rekonstruktion allein aus binaeren Authentisierungsergebnissen geschuetzter Templates (Jan. 2026) — [arXiv 2601.17620](https://arxiv.org/abs/2601.17620?context=cs)
- BioZKFHE: Rechenknoten liefert pro Block oeffentlich pruefbaren Beweis (Schutz gegen boesartigen Server), Threshold-Komitee entschluesselt nur Urteil, aber "honest-client"-Annahme und Galerie-Snapshot — [arXiv 2611.12636](https://arxiv.org/pdf/2611.12636); [arXiv 2607.22065](https://arxiv.org/pdf/2607.22065)
- BioBind: Bedingungen I1–I4 binden Dedup-Anfrage, Galerie-Einfuegung und Commitment an denselben Vektor (gegen "pruefe A, fuege B ein"), authentisierte Galerie-Updates — [arXiv 2611.12636](https://arxiv.org/html/2611.12636)
- Biometrische Fehlerraten begrenzen Einmaligkeit: 1,8 % erfolgreiche Umgehung durch Aussehensaenderung (Gesicht, Bosphorus) — [arXiv 2611.12636](https://arxiv.org/pdf/2611.12636)
- Regulatorische Zweifel an nicht nachpruefbarer Loeschung — [Wikipedia: World (blockchain)](https://en.wikipedia.org/wiki/World_(blockchain))

### Inferences
- Wiederholte Abfragen (Hill-Climbing) gegen ein Dedup-Orakel sind auch bei binaerer Ausgabe ein Angriffsvektor → Rate-Limits und Bindung jeder Abfrage an echte, attestierte Erfassung (Enrollment-Station/Orb) sind Teil der Sicherheitsarchitektur, nicht optional.
- Fuer "kein einzelner Akteur haelt nutzbare Biometrie" reicht 3-Parteien-honest-majority nur, wenn die Betreiber rechtlich/organisatorisch unabhaengig sind (World: FAU, Berkeley, KAIST, ggf. Nethermind) und nicht derselben Jurisdiktion/Weisung unterliegen; malicious-sichere Varianten (MAC-basiert, z.B. SPDZ-Familie) kosten typischerweise ein Mehrfaches an Rechenzeit.
- Galerie-Integritaet: Ohne Bindung (BioBind I1–I4) kann ein boesartiger Betreiber Eintraege weglassen (Dubletten zulassen) oder fremde einfuegen (Personen aussperren) — das ist mit Vertraulichkeitsschutz allein nicht abgedeckt.

### Gaps
- Keine Quelle zu einem malicious-sicheren, produktiv betriebenen Iris-MPC gefunden.
- Keine quantitative Studie gefunden, wie viele binaere Abfragen fuer eine Iris-Code-Rekonstruktion noetig sind (arXiv 2601.17620 nicht im Volltext gelesen).
