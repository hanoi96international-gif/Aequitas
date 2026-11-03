# Umzug in eine GitHub-Organisation

Stand: 03.11.2026.

## Warum

Heute liegen alle vier Repos, alle Server-Schlüssel (als Secrets) und jede
Freigabe für C1 an **einem** privaten Konto. Wer dieses Konto übernimmt,
etwa über ein gestohlenes Handy oder ein Phishing, kontrolliert die Kette,
den Coordinator und die App-Signatur. Eine Organisation mit zwei Inhabern
und Pflicht-2FA nimmt diesen einen Punkt weg.

## Reihenfolge

Die Schritte 1 bis 3 ändern nichts am Betrieb und gehen sofort.

1. **Organisation anlegen** (kostenlos): github.com → „New organization“ →
   Free. Als Name am besten den der künftigen Rechtsform, z. B.
   `aequitas-digital`.
2. **Zwei-Faktor-Pflicht** einschalten: Settings → Authentication security →
   „Require two-factor authentication“. Vorher bei beiden Inhabern einen
   Passkey oder eine Authenticator-App einrichten, nicht SMS.
3. **Zweiten Inhaber** einladen (Role: Owner): eine Person, der du die Kette
   anvertraust, idealerweise der zweite Betreiber.
4. **Repos übertragen**, in dieser Reihenfolge:
   `aequitas-proof-server`, `aequitas-biometric-beta`, `Aequitas-App`,
   zuletzt `Aequitas`. Je Repo: Settings → Danger Zone → Transfer.
5. **Direkt danach den Umzugs-PR** einspielen (Abschnitt „Was ich danach
   ändere“). Bis er gemergt ist, kein Deploy starten.

## Was beim Übertragen mitgeht und was nicht

| Ding | Geht mit? | Folge |
|---|---|---|
| Code, Issues, PRs, Releases | ja | — |
| Secrets und Umgebungen (`produktion`, `app-signatur`) | ja, bleiben am Repo | Prüfen, dass die Freigabe-Regel noch auf deinen Nutzer zeigt |
| Git- und Web-Adressen `github.com/hanoi96international-gif/...` | werden umgeleitet | Auch Downloads von Releases. Die Umleitung hält nur, solange **kein neues Repo mit dem alten Namen** unter dem alten Konto angelegt wird |
| Container-Images `ghcr.io/hanoi96international-gif/...` | **werden nicht umgeleitet** | Siehe unten |
| Paketzugriff (Aequitas darf die Images der anderen Repos ziehen) | **neu setzen** | Package settings → „Manage Actions access“ → Aequitas hinzufügen |
| GitHub Pages | ja, aber neue Adresse | `<org>.github.io/...` statt `hanoi96international-gif.github.io/...` |
| Go-Modulpfad `github.com/hanoi96international-gif/aequitas-chain` | — | Bleibt so. Es ist nur ein Name, der Bau holt nichts von dort |

## Die Images: der einzige heikle Punkt

Die Bau-Workflows schreiben nach `ghcr.io/${{ github.repository }}/...`. Nach
dem Umzug landen neue Images also von selbst unter der Organisation. Die
Deploy-Wege in Aequitas nennen den alten Pfad aber fest: 9 Stellen in sechs
Workflows und zwei Skripten unter `deploy/`, z. B.
`bio-deploy-coordinator-to-box.yml` und `proof-server-ausrollen.yml`.
Die schon laufenden Container auf C1 sind nicht betroffen: Ihre Images
liegen lokal.

Deshalb:
1. Nach dem Übertragen in beiden Repos (`aequitas-biometric-beta`,
   `aequitas-proof-server`) einmal den Bau starten. Dann gibt es den
   aktuellen Stand unter dem neuen Pfad.
2. Den Paketzugriff für Aequitas neu setzen (Tabelle oben).
3. Erst dann den Umzugs-PR mergen und einmal neu ausrollen, als Probe.

## Was ich danach ändere (ein PR je Repo)

- Aequitas: die 9 Image-Pfade samt Test `scripts/test_proof_ausrollen.sh`;
  `defaultAPKReleaseURL` (`x/humanity/keeper/api.go`); Clone-Adressen in
  `deploy/` und `docs/`.
- Aequitas-App: `GITHUB_URL` in `app/(tabs)/run-node.tsx`; `$Repo` in
  `scripts/signaturschluessel-anlegen.ps1`.
- aequitas-proof-server, aequitas-biometric-beta: Verweise in der Doku.

Schon installierte Apps holen Updates über `aequitas.digital/download/app.apk`.
Das läuft über den Knoten und hängt nicht am Kontonamen.

## Danach

- Das private Konto behält nur noch die Mitgliedschaft in der Organisation.
- Deploy-Freigaben (`produktion`) auf beide Inhaber setzen, damit eine
  Freigabe nicht an einer Person hängt.
- Die App-Signatur (`app-signatur`) nur einer Person geben. Der
  Signaturschlüssel bleibt zusätzlich offline gesichert
  (`docs/RELEASE_SIGNING.md` im App-Repo).
