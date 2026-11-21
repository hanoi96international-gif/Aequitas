#!/usr/bin/env python3
"""Aequitas-Validator-Anleitung als PDF in 12 Sprachen.

Quelle des Inhalts: docs/VALIDATOR_EINRICHTEN.md (Stand der Einrichtung mit
einem Befehl). Die Mindestwerte (Kerne, Arbeitsspeicher, Platz, Signaturen je
Sekunde, Commit-Zeit) liest dieses Skript aus deploy/validator/einrichten.sh --
dieselben Zahlen, mit denen das Skript einen Server ablehnt. So koennen PDF
und Pruefung nicht auseinanderlaufen.

Aufruf (aus dem Hauptordner):
    python3 build/node_guide.py            # HTML nach build/node_guide_html/
    node build/node_guide_pdf.cjs          # PDFs nach downloads/ und docs/

Gedruckt wird mit Chromium (Playwright), damit Arabisch (von rechts nach links),
Hindi und Chinesisch richtig gesetzt werden. Gebraucht werden die Noto-Schriften
(Debian/Ubuntu: fonts-noto-core fonts-noto-cjk); die PDFs betten sie ein.

Die Website liefert die PDFs nicht aus: /download/node-guide-xx.pdf leitet auf
die Anleitung im Explorer um (/network/node), die mit jedem Deploy aktuell ist.
"""
import html
import os
import re
import sys

HIER = os.path.dirname(os.path.abspath(__file__))
WURZEL = os.path.dirname(HIER)
AUS = os.path.join(HIER, "node_guide_html")


def mindestwerte():
    """Die Grenzen, mit denen einrichten.sh einen Server ablehnt."""
    s = open(os.path.join(WURZEL, "deploy/validator/einrichten.sh"), encoding="utf-8").read()
    m = re.search(r"MIN_KERNE=(\d+); MIN_RAM_MB=(\d+); MIN_PLATZ_GB=(\d+)", s)
    r = re.search(r"\$MIN_KERNE Kerne/cores, (\d+) GB RAM", s)
    p = re.search(r"mindestens (\d+) Signaturen/s, Datenbank-Commit hoechstens (\d+) ms", s)
    if not (m and r and p):
        sys.exit("einrichten.sh: Mindestwerte nicht gefunden -- Anleitung nicht erzeugt")
    return {
        "kerne": int(m.group(1)), "ram": int(r.group(1)), "platz": int(m.group(3)),
        "sig": int(p.group(1)), "ms": int(p.group(2)),
    }


BEFEHLE_EINRICHTEN = """ssh root@SERVER-IP
curl -fsSL https://get.docker.com | sh
git clone https://github.com/hanoi96international-gif/Aequitas.git
cd Aequitas/deploy/validator && bash einrichten.sh"""
BEFEHLE_LAEUFT = """curl -s http://localhost:8080/api/status | grep -oE '"height":[0-9]+'
curl -s https://aequitas.digital/api/status | grep -oE '"height":[0-9]+'
docker compose logs -f node"""
BEFEHL_AKTUALISIEREN = "cd ~/Aequitas && git pull && cd deploy/validator && docker compose up -d --build"
BEFEHLE_QUANTEN = """cd ~/Aequitas/deploy/validator
bash quanten-einrichten.sh --automatisch
docker compose -f docker-compose.yml -f docker-compose.quanten.yml up -d --build"""

# Je Sprache: Texte. {kerne} {ram} {platz} {sig} {ms} werden ersetzt.
# Listen: Abschnitte; Tabellen als Zeilen.
T = {}

T["de"] = dict(
    titel="Einen Aequitas-Validator betreiben",
    unter="Anleitung für Betreiber · Stand 21.11.2026 · aequitas.digital/network/node",
    was_h="Worum es geht",
    was="Ein Validator ist ein Rechner, der die Kette mitführt, jeden Block selbst nachprüft und selbst Blöcke erzeugt. Dafür bekommt der Mensch, dem er gehört, jeden Tag einen gleichen Anteil am Validator-Topf. Ein Mensch kann genau einen Validator betreiben; mehr Rechenleistung bringt keine zusätzliche Stimme.",
    brauchst_h="Was du brauchst",
    tab_kopf=("", "Mindestens"),
    tab=[
        ("Server", "gemieteter Server (VPS oder Root-Server), Ubuntu 22.04 oder 24.04, öffentliche IPv4-Adresse"),
        ("Prozessor", "<b>{kerne} dedizierte Kerne</b> einer aktuellen Generation (z. B. AMD EPYC 9004/9005, Ryzen 7000/9000, Intel Xeon 4. Gen.)"),
        ("Arbeitsspeicher", "<b>{ram} GB</b>"),
        ("Festplatte", "<b>{platz} GB NVMe-SSD</b> frei"),
        ("Offene Ports", "8080 (Schnittstelle) und 4001 (Verbindung zu anderen Knoten)"),
        ("Leistung", "<b>{sig} Signaturen je Sekunde</b>, Datenbank-Commit höchstens {ms} ms"),
        ("Du selbst", "in der Aequitas-App registriert (Gesichtsprüfung abgeschlossen)"),
    ],
    kosten="So ein Server kostet bei den meisten Anbietern etwa <b>20 bis 40 Euro im Monat</b>. Wähle „8 vCPU dediziert“ oder einen Root-Server, <b>nicht</b> die kleinste Klasse mit geteilten vCPU.",
    pruef_h="Zu schwache Hardware wird abgelehnt – an drei Stellen",
    pruef=[
        "Das Einrichtungsskript prüft vor dem Bauen Kerne, Arbeitsspeicher und Platz.",
        "Der Knoten misst sich selbst (Signaturen je Sekunde, Datenbank). Reicht es nicht, hält das Skript ihn an.",
        "Das Netz misst ihn bei der Anmeldung noch einmal mit einer Rechenaufgabe aus einem Zufallswert. Diese Probe kann ein Server nicht schönreden.",
    ],
    nicht="<b>Nicht auf den Server gehören</b> deine Wallet, ihre Wörterliste (Seed-Phrase) und ihr privater Schlüssel. Auf dem Server liegt nur die <b>Adresse</b> deiner Wallet.",
    einr_h="Einrichten: ein Befehl, zwei Fragen",
    einr="Mit dem Server verbinden (Windows: PowerShell, Mac: Terminal) und diese Zeilen einfügen:",
    einr2="Das Skript fragt nur nach <b>deiner Wallet-Adresse</b> (die Adresse aus der App, beginnt mit 0x) und nach der <b>Bestätigung der IP-Adresse</b>. Alles andere erledigt es selbst: Datenbank-Passwort, Konfiguration, Bauen und Starten (beim ersten Mal etwa 10 Minuten) und das Sichern der Schlüssel des Knotens.",
    app_h="In der App bestätigen",
    app=[
        "Das Skript zeigt am Ende eine sechsstellige <b>Kontrollzahl</b>.",
        "Aequitas-App → Tab <b>„Knoten“</b>: „Dein Server möchte sich mit dir verbinden“ mit einer Zahl.",
        "Nur wenn die Zahl <b>genau</b> gleich ist, auf <b>„Bestätigen“</b> tippen. Die Unterschriften kosten nichts und bewegen kein Geld.",
    ],
    app2="Ohne App (z. B. MetaMask): http://SERVER-IP:8080/node-binding öffnen und mit derselben Wallet unterschreiben.",
    aufn_h="Danach: Aufnahme als Blockproduzent",
    aufn="Heute nimmt der Betreiber neue Blockproduzenten noch von Hand auf, bis jeder Knoten jeden Wert eines Blocks selbst nachrechnet. Schick ihm deine Signieradresse. Bis dahin läuft dein Knoten als vollwertiger Beobachter und prüft jeden Block selbst.",
    laeuft_h="Läuft er? Aktualisieren",
    laeuft="Beide Höhen sollten bis auf ein paar Blöcke gleich sein. Das Log beendest du mit Strg+C; der Knoten läuft weiter. Aktualisieren:",
    wo_h="Was wo liegt",
    wo_kopf=("", "Wo"),
    wo=[
        ("Signierschlüssel, P2P-Schlüssel", "nur auf dem Server, in .env"),
        ("Adresse deiner Wallet", "auf dem Server (dorthin gehen die Belohnungen)"),
        ("IBM-Quantum-Schlüssel (nur mit Quantengeld)", "nur auf dem Server, nur für root lesbar"),
        ("Wallet, Wörterliste, privater Schlüssel", "<b>nur bei dir</b>, nie auf dem Server"),
    ],
    wo2="<b>Sichere die Datei .env.</b> Geht sie verloren, bekommt der Knoten eine neue Identität und muss neu gebunden und aufgenommen werden.",
    fehler_h="Wenn etwas nicht geht",
    fehler_kopf=("Meldung", "Was tun"),
    fehler=[
        ("NODE_OPERATOR_WALLET is not a registered human", "erst in der App registrieren"),
        ("„Keine Bestätigung aus der App angekommen“", "bash einrichten.sh erneut starten und in der App unter „Knoten“ bestätigen"),
        ("„Dieser Server ist zu schwach“", "größeren Server nehmen"),
        ("„Leistung nicht ausreichend“", "schnelleren Server nehmen (aktuelle CPU, NVMe)"),
        ("„Leistungsprobe des Netzes NICHT bestanden“", "Grund steht dabei; bei „nicht erreichbar“ Port 8080 öffnen"),
        ("registriert, aber nicht in AUTHORIZED_VALIDATORS", "noch nicht aufgenommen; Signieradresse an den Betreiber schicken"),
        ("Höhe bleibt hinter dem Netz zurück", "Ports 4001/8080 beim Anbieter öffnen, Arbeitsspeicher prüfen (docker stats)"),
    ],
    q_h="Quantengeld (freiwillig)",
    q="Mit einem eigenen Konto bei <b>IBM Quantum</b> präpariert dein Knoten bei einer Ausgabe von Quantengeld deinen eigenen Abschnitt; dann kennt niemand außer dir dessen Zustände. Zusätzliche Hardware brauchst du nicht – die Messung läuft über IBM, Skripte und Python auf demselben Server. Der kostenlose IBM-Plan reicht für den Anfang (begrenzte Rechenzeit im Monat). Einmal als root:",
    q2="Das Skript fragt einmal nach dem API-Schlüssel (nicht angezeigt, nie im Log). Danach misst ein Timer Vormerkungen automatisch, höchstens 8 am Tag (einstellbar mit QS_JE_TAG).",
    fuss="Ausführlich und immer aktuell: aequitas.digital/network/node · docs/VALIDATOR_EINRICHTEN.md im Repository. Fragen: Telegram-Gruppe auf aequitas.digital.",
)

T["en"] = dict(
    titel="Running an Aequitas validator",
    unter="Operator guide · as of 21 Nov 2026 · aequitas.digital/network/node",
    was_h="What it is",
    was="A validator is a computer that keeps the chain, checks every block itself and produces blocks itself. In return, the human who owns it receives an equal daily share of the validator pool. One human can run exactly one validator; more computing power gives no extra vote.",
    brauchst_h="What you need",
    tab_kopf=("", "Minimum"),
    tab=[
        ("Server", "rented server (VPS or root server), Ubuntu 22.04 or 24.04, public IPv4 address"),
        ("Processor", "<b>{kerne} dedicated cores</b> of a current generation (e.g. AMD EPYC 9004/9005, Ryzen 7000/9000, Intel Xeon 4th gen.)"),
        ("Memory", "<b>{ram} GB</b>"),
        ("Disk", "<b>{platz} GB NVMe SSD</b> free"),
        ("Open ports", "8080 (API) and 4001 (connection to other nodes)"),
        ("Performance", "<b>{sig} signatures per second</b>, database commit at most {ms} ms"),
        ("You", "registered in the Aequitas app (face check completed)"),
    ],
    kosten="Such a server costs about <b>20 to 40 euros a month</b> at most providers. Choose “8 vCPU dedicated” or a root server, <b>not</b> the smallest class with shared vCPUs.",
    pruef_h="Hardware that is too weak is rejected – at three points",
    pruef=[
        "The setup script checks cores, memory and space before building.",
        "The node measures itself (signatures per second, database). If it falls short, the script stops it.",
        "The network measures it again when it joins, with a task derived from a random value. A server cannot fake this test.",
    ],
    nicht="<b>Never put on the server</b> your wallet, its word list (seed phrase) or its private key. Only your wallet's <b>address</b> goes on the server.",
    einr_h="Setup: one command, two questions",
    einr="Connect to the server (Windows: PowerShell, Mac: Terminal) and paste these lines:",
    einr2="The script only asks for <b>your wallet address</b> (the address from the app, starting with 0x) and to <b>confirm the IP address</b>. It does everything else itself: database password, configuration, building and starting (about 10 minutes the first time) and saving the node's keys.",
    app_h="Confirm in the app",
    app=[
        "At the end the script shows a six-digit <b>check number</b>.",
        "Aequitas app → tab <b>“Node”</b>: “Your server wants to connect to you” with a number.",
        "Only if the number is <b>exactly</b> the same, tap <b>“Confirm”</b>. The signatures cost nothing and move no money.",
    ],
    app2="Without the app (e.g. MetaMask): open http://SERVER-IP:8080/node-binding and sign with the same wallet.",
    aufn_h="Afterwards: admission as a block producer",
    aufn="For now the operator still admits new block producers by hand, until every node recomputes every value of a block itself. Send them your signing address. Until then your node runs as a full observer and checks every block itself.",
    laeuft_h="Is it running? Updating",
    laeuft="Both heights should match within a few blocks. Ctrl+C ends the log view; the node keeps running. To update:",
    wo_h="What is stored where",
    wo_kopf=("", "Where"),
    wo=[
        ("Signing key, P2P key", "only on the server, in .env"),
        ("Your wallet address", "on the server (rewards go there)"),
        ("IBM Quantum key (only with quantum money)", "only on the server, readable by root only"),
        ("Wallet, word list, private key", "<b>only with you</b>, never on the server"),
    ],
    wo2="<b>Back up the .env file.</b> If it is lost, the node gets a new identity and must be bound and admitted again.",
    fehler_h="If something goes wrong",
    fehler_kopf=("Message", "What to do"),
    fehler=[
        ("NODE_OPERATOR_WALLET is not a registered human", "register in the app first"),
        ("“No confirmation from the app”", "run bash einrichten.sh again and confirm in the app under “Node”"),
        ("“This server is too weak”", "use a bigger server"),
        ("“Performance not sufficient”", "use a faster server (current CPU, NVMe)"),
        ("“Network performance test NOT passed”", "the reason is shown; if “not reachable”, open port 8080"),
        ("registered, but not in AUTHORIZED_VALIDATORS", "not admitted yet; send your signing address to the operator"),
        ("Height stays behind the network", "open ports 4001/8080 at your provider, check memory (docker stats)"),
    ],
    q_h="Quantum money (optional)",
    q="With your own <b>IBM Quantum</b> account, your node prepares your own section when quantum money is issued; then nobody but you knows its states. You need no extra hardware – the measurement runs at IBM, scripts and Python on the same server. IBM's free plan is enough to start (limited computing time per month). Once, as root:",
    q2="The script asks once for the API key (not shown, never logged). Afterwards a timer measures pre-booked jobs automatically, at most 8 a day (adjustable with QS_JE_TAG).",
    fuss="In full and always current: aequitas.digital/network/node · docs/VALIDATOR_EINRICHTEN.md in the repository. Questions: Telegram group on aequitas.digital.",
)

# Die Meldungen des Skripts sind deutsch/englisch; in den anderen Sprachen
# steht die englische Meldung, die Hilfe ist uebersetzt.
_MELD = [m for m, _ in T["en"]["fehler"]]


def _fehler(hilfen):
    return list(zip(_MELD, hilfen))


T["es"] = dict(
    titel="Operar un validador de Aequitas",
    unter="Guía para operadores · estado 21/11/2026 · aequitas.digital/network/node",
    was_h="De qué se trata",
    was="Un validador es un ordenador que lleva la cadena, comprueba cada bloque por sí mismo y produce bloques. A cambio, la persona a la que pertenece recibe cada día una parte igual del fondo de validadores. Una persona puede operar exactamente un validador; más potencia de cálculo no da ningún voto adicional.",
    brauchst_h="Qué necesitas",
    tab_kopf=("", "Mínimo"),
    tab=[
        ("Servidor", "servidor alquilado (VPS o servidor dedicado), Ubuntu 22.04 o 24.04, dirección IPv4 pública"),
        ("Procesador", "<b>{kerne} núcleos dedicados</b> de generación actual (p. ej. AMD EPYC 9004/9005, Ryzen 7000/9000, Intel Xeon 4.ª gen.)"),
        ("Memoria", "<b>{ram} GB</b>"),
        ("Disco", "<b>{platz} GB SSD NVMe</b> libres"),
        ("Puertos abiertos", "8080 (API) y 4001 (conexión con otros nodos)"),
        ("Rendimiento", "<b>{sig} firmas por segundo</b>, commit de la base de datos como máximo {ms} ms"),
        ("Tú", "registrado en la app de Aequitas (verificación facial completada)"),
    ],
    kosten="Un servidor así cuesta unos <b>20 a 40 euros al mes</b> en la mayoría de proveedores. Elige «8 vCPU dedicadas» o un servidor dedicado, <b>no</b> la clase más pequeña con vCPU compartidas.",
    pruef_h="El hardware demasiado débil se rechaza, en tres puntos",
    pruef=[
        "El script de instalación comprueba núcleos, memoria y espacio antes de compilar.",
        "El nodo se mide a sí mismo (firmas por segundo, base de datos). Si no alcanza, el script lo detiene.",
        "La red lo vuelve a medir al unirse, con una tarea derivada de un valor aleatorio. Un servidor no puede falsear esta prueba.",
    ],
    nicht="<b>Nunca van al servidor</b> tu monedero, su lista de palabras (frase semilla) ni su clave privada. En el servidor solo está la <b>dirección</b> de tu monedero.",
    einr_h="Instalación: un comando, dos preguntas",
    einr="Conéctate al servidor (Windows: PowerShell, Mac: Terminal) y pega estas líneas:",
    einr2="El script solo pregunta por <b>la dirección de tu monedero</b> (la de la app, empieza por 0x) y pide <b>confirmar la dirección IP</b>. Todo lo demás lo hace solo: contraseña de la base de datos, configuración, compilar y arrancar (unos 10 minutos la primera vez) y guardar las claves del nodo.",
    app_h="Confirmar en la app",
    app=[
        "Al final el script muestra un <b>número de control</b> de seis cifras.",
        "App de Aequitas → pestaña <b>«Nodo»</b>: «Tu servidor quiere conectarse contigo» con un número.",
        "Solo si el número es <b>exactamente</b> igual, pulsa <b>«Confirmar»</b>. Las firmas no cuestan nada y no mueven dinero.",
    ],
    app2="Sin la app (p. ej. MetaMask): abre http://SERVER-IP:8080/node-binding y firma con el mismo monedero.",
    aufn_h="Después: admisión como productor de bloques",
    aufn="Por ahora el operador admite a mano a los nuevos productores de bloques, hasta que cada nodo recalcule por sí mismo cada valor de un bloque. Envíale tu dirección de firma. Hasta entonces tu nodo funciona como observador completo y comprueba cada bloque.",
    laeuft_h="¿Funciona? Actualizar",
    laeuft="Ambas alturas deberían coincidir con unos pocos bloques de diferencia. Ctrl+C cierra el registro; el nodo sigue funcionando. Para actualizar:",
    wo_h="Qué está dónde",
    wo_kopf=("", "Dónde"),
    wo=[
        ("Clave de firma, clave P2P", "solo en el servidor, en .env"),
        ("Dirección de tu monedero", "en el servidor (allí llegan las recompensas)"),
        ("Clave de IBM Quantum (solo con dinero cuántico)", "solo en el servidor, legible solo por root"),
        ("Monedero, lista de palabras, clave privada", "<b>solo contigo</b>, nunca en el servidor"),
    ],
    wo2="<b>Haz copia del archivo .env.</b> Si se pierde, el nodo recibe una identidad nueva y hay que vincularlo y admitirlo de nuevo.",
    fehler_h="Si algo falla",
    fehler_kopf=("Mensaje", "Qué hacer"),
    fehler=_fehler([
        "primero regístrate en la app",
        "vuelve a ejecutar bash einrichten.sh y confirma en la app en «Nodo»",
        "usa un servidor más grande",
        "usa un servidor más rápido (CPU actual, NVMe)",
        "se indica el motivo; si «no accesible», abre el puerto 8080",
        "aún no admitido; envía tu dirección de firma al operador",
        "abre los puertos 4001/8080 en tu proveedor, revisa la memoria (docker stats)",
    ]),
    q_h="Dinero cuántico (opcional)",
    q="Con tu propia cuenta de <b>IBM Quantum</b>, tu nodo prepara tu propia sección cuando se emite dinero cuántico; así nadie más que tú conoce sus estados. No necesitas hardware adicional: la medición se hace en IBM, los scripts y Python en el mismo servidor. El plan gratuito de IBM basta para empezar (tiempo de cálculo limitado al mes). Una vez, como root:",
    q2="El script pide una vez la clave de API (no se muestra, nunca en el registro). Después un temporizador mide automáticamente lo reservado, como máximo 8 al día (ajustable con QS_JE_TAG).",
    fuss="Completo y siempre actual: aequitas.digital/network/node · docs/VALIDATOR_EINRICHTEN.md en el repositorio. Preguntas: grupo de Telegram en aequitas.digital.",
)

T["fr"] = dict(
    titel="Exploiter un validateur Aequitas",
    unter="Guide de l’opérateur · état au 21/11/2026 · aequitas.digital/network/node",
    was_h="De quoi il s’agit",
    was="Un validateur est un ordinateur qui tient la chaîne, vérifie lui-même chaque bloc et produit des blocs. En échange, la personne à qui il appartient reçoit chaque jour une part égale du fonds des validateurs. Une personne peut exploiter exactement un validateur ; plus de puissance de calcul ne donne aucune voix de plus.",
    brauchst_h="Ce qu’il te faut",
    tab_kopf=("", "Au minimum"),
    tab=[
        ("Serveur", "serveur loué (VPS ou serveur dédié), Ubuntu 22.04 ou 24.04, adresse IPv4 publique"),
        ("Processeur", "<b>{kerne} cœurs dédiés</b> de génération actuelle (p. ex. AMD EPYC 9004/9005, Ryzen 7000/9000, Intel Xeon 4ᵉ gén.)"),
        ("Mémoire", "<b>{ram} Go</b>"),
        ("Disque", "<b>{platz} Go de SSD NVMe</b> libres"),
        ("Ports ouverts", "8080 (API) et 4001 (connexion aux autres nœuds)"),
        ("Performance", "<b>{sig} signatures par seconde</b>, commit de la base de données en {ms} ms au plus"),
        ("Toi", "inscrit dans l’application Aequitas (contrôle du visage terminé)"),
    ],
    kosten="Un tel serveur coûte environ <b>20 à 40 euros par mois</b> chez la plupart des hébergeurs. Choisis « 8 vCPU dédiés » ou un serveur dédié, <b>pas</b> la plus petite offre à vCPU partagés.",
    pruef_h="Un matériel trop faible est refusé – à trois endroits",
    pruef=[
        "Le script d’installation vérifie cœurs, mémoire et espace avant la compilation.",
        "Le nœud se mesure lui-même (signatures par seconde, base de données). Si cela ne suffit pas, le script l’arrête.",
        "Le réseau le mesure encore à l’entrée, avec une tâche tirée d’une valeur aléatoire. Un serveur ne peut pas truquer ce test.",
    ],
    nicht="<b>Ne mets jamais sur le serveur</b> ton portefeuille, sa liste de mots (phrase de récupération) ni sa clé privée. Seule l’<b>adresse</b> de ton portefeuille y figure.",
    einr_h="Installation : une commande, deux questions",
    einr="Connecte-toi au serveur (Windows : PowerShell, Mac : Terminal) et colle ces lignes :",
    einr2="Le script demande seulement <b>l’adresse de ton portefeuille</b> (celle de l’application, commençant par 0x) et de <b>confirmer l’adresse IP</b>. Il fait tout le reste : mot de passe de la base, configuration, compilation et démarrage (environ 10 minutes la première fois) et sauvegarde des clés du nœud.",
    app_h="Confirmer dans l’application",
    app=[
        "À la fin, le script affiche un <b>nombre de contrôle</b> à six chiffres.",
        "Application Aequitas → onglet <b>« Nœud »</b> : « Ton serveur veut se connecter à toi » avec un nombre.",
        "Uniquement si le nombre est <b>exactement</b> le même, touche <b>« Confirmer »</b>. Les signatures ne coûtent rien et ne déplacent pas d’argent.",
    ],
    app2="Sans l’application (p. ex. MetaMask) : ouvre http://SERVER-IP:8080/node-binding et signe avec le même portefeuille.",
    aufn_h="Ensuite : admission comme producteur de blocs",
    aufn="Pour l’instant, l’opérateur admet encore à la main les nouveaux producteurs de blocs, jusqu’à ce que chaque nœud recalcule lui-même chaque valeur d’un bloc. Envoie-lui ton adresse de signature. D’ici là, ton nœud tourne comme observateur complet et vérifie chaque bloc.",
    laeuft_h="Est-ce qu’il tourne ? Mettre à jour",
    laeuft="Les deux hauteurs doivent être égales à quelques blocs près. Ctrl+C ferme l’affichage du journal ; le nœud continue. Pour mettre à jour :",
    wo_h="Où se trouve quoi",
    wo_kopf=("", "Où"),
    wo=[
        ("Clé de signature, clé P2P", "uniquement sur le serveur, dans .env"),
        ("Adresse de ton portefeuille", "sur le serveur (les récompenses y vont)"),
        ("Clé IBM Quantum (seulement avec la monnaie quantique)", "uniquement sur le serveur, lisible par root seulement"),
        ("Portefeuille, liste de mots, clé privée", "<b>seulement chez toi</b>, jamais sur le serveur"),
    ],
    wo2="<b>Sauvegarde le fichier .env.</b> S’il est perdu, le nœud reçoit une nouvelle identité et doit être lié et admis à nouveau.",
    fehler_h="Si quelque chose ne va pas",
    fehler_kopf=("Message", "Que faire"),
    fehler=_fehler([
        "d’abord s’inscrire dans l’application",
        "relancer bash einrichten.sh et confirmer dans l’application sous « Nœud »",
        "prendre un serveur plus grand",
        "prendre un serveur plus rapide (CPU actuel, NVMe)",
        "la raison est indiquée ; si « injoignable », ouvrir le port 8080",
        "pas encore admis ; envoyer l’adresse de signature à l’opérateur",
        "ouvrir les ports 4001/8080 chez l’hébergeur, vérifier la mémoire (docker stats)",
    ]),
    q_h="Monnaie quantique (facultatif)",
    q="Avec ton propre compte <b>IBM Quantum</b>, ton nœud prépare ta propre section lors d’une émission de monnaie quantique ; personne d’autre que toi n’en connaît alors les états. Aucun matériel supplémentaire n’est nécessaire : la mesure se fait chez IBM, les scripts et Python tournent sur le même serveur. L’offre gratuite d’IBM suffit pour commencer (temps de calcul limité par mois). Une fois, en root :",
    q2="Le script demande une fois la clé d’API (non affichée, jamais dans le journal). Ensuite un minuteur mesure automatiquement ce qui est réservé, au plus 8 fois par jour (réglable avec QS_JE_TAG).",
    fuss="En détail et toujours à jour : aequitas.digital/network/node · docs/VALIDATOR_EINRICHTEN.md dans le dépôt. Questions : groupe Telegram sur aequitas.digital.",
)

T["it"] = dict(
    titel="Gestire un validatore Aequitas",
    unter="Guida per gestori · aggiornata al 21/11/2026 · aequitas.digital/network/node",
    was_h="Di che cosa si tratta",
    was="Un validatore è un computer che tiene la catena, verifica da sé ogni blocco e produce blocchi. In cambio la persona a cui appartiene riceve ogni giorno una quota uguale del fondo dei validatori. Una persona può gestire esattamente un validatore; più potenza di calcolo non dà alcun voto in più.",
    brauchst_h="Che cosa ti serve",
    tab_kopf=("", "Come minimo"),
    tab=[
        ("Server", "server a noleggio (VPS o server dedicato), Ubuntu 22.04 o 24.04, indirizzo IPv4 pubblico"),
        ("Processore", "<b>{kerne} core dedicati</b> di generazione attuale (ad es. AMD EPYC 9004/9005, Ryzen 7000/9000, Intel Xeon 4ª gen.)"),
        ("Memoria", "<b>{ram} GB</b>"),
        ("Disco", "<b>{platz} GB di SSD NVMe</b> liberi"),
        ("Porte aperte", "8080 (API) e 4001 (connessione agli altri nodi)"),
        ("Prestazioni", "<b>{sig} firme al secondo</b>, commit del database al massimo {ms} ms"),
        ("Tu", "registrato nell'app Aequitas (verifica del volto completata)"),
    ],
    kosten="Un server così costa circa <b>da 20 a 40 euro al mese</b> presso la maggior parte dei fornitori. Scegli «8 vCPU dedicate» o un server dedicato, <b>non</b> la classe più piccola con vCPU condivise.",
    pruef_h="L'hardware troppo debole viene respinto – in tre punti",
    pruef=[
        "Lo script di installazione controlla core, memoria e spazio prima della compilazione.",
        "Il nodo misura sé stesso (firme al secondo, database). Se non basta, lo script lo ferma.",
        "La rete lo misura di nuovo all'ingresso, con un compito ricavato da un valore casuale. Un server non può falsare questa prova.",
    ],
    nicht="<b>Non vanno mai sul server</b> il tuo portafoglio, la sua lista di parole (frase seed) né la sua chiave privata. Sul server c'è solo l'<b>indirizzo</b> del tuo portafoglio.",
    einr_h="Installazione: un comando, due domande",
    einr="Collegati al server (Windows: PowerShell, Mac: Terminale) e incolla queste righe:",
    einr2="Lo script chiede solo <b>l'indirizzo del tuo portafoglio</b> (quello dell'app, inizia con 0x) e di <b>confermare l'indirizzo IP</b>. Tutto il resto lo fa da sé: password del database, configurazione, compilazione e avvio (circa 10 minuti la prima volta) e salvataggio delle chiavi del nodo.",
    app_h="Confermare nell'app",
    app=[
        "Alla fine lo script mostra un <b>numero di controllo</b> di sei cifre.",
        "App Aequitas → scheda <b>«Nodo»</b>: «Il tuo server vuole collegarsi a te» con un numero.",
        "Solo se il numero è <b>esattamente</b> uguale, tocca <b>«Conferma»</b>. Le firme non costano nulla e non spostano denaro.",
    ],
    app2="Senza l'app (ad es. MetaMask): apri http://SERVER-IP:8080/node-binding e firma con lo stesso portafoglio.",
    aufn_h="Dopo: ammissione come produttore di blocchi",
    aufn="Per ora il gestore ammette ancora a mano i nuovi produttori di blocchi, finché ogni nodo non ricalcola da sé ogni valore di un blocco. Inviagli il tuo indirizzo di firma. Fino ad allora il tuo nodo funziona come osservatore completo e verifica ogni blocco.",
    laeuft_h="Funziona? Aggiornare",
    laeuft="Le due altezze dovrebbero coincidere a meno di pochi blocchi. Ctrl+C chiude il log; il nodo continua a funzionare. Per aggiornare:",
    wo_h="Che cosa si trova dove",
    wo_kopf=("", "Dove"),
    wo=[
        ("Chiave di firma, chiave P2P", "solo sul server, in .env"),
        ("Indirizzo del tuo portafoglio", "sul server (lì arrivano le ricompense)"),
        ("Chiave IBM Quantum (solo con il denaro quantistico)", "solo sul server, leggibile solo da root"),
        ("Portafoglio, lista di parole, chiave privata", "<b>solo da te</b>, mai sul server"),
    ],
    wo2="<b>Fai una copia del file .env.</b> Se va perso, il nodo riceve una nuova identità e va collegato e ammesso di nuovo.",
    fehler_h="Se qualcosa non va",
    fehler_kopf=("Messaggio", "Che cosa fare"),
    fehler=_fehler([
        "prima registrarsi nell'app",
        "riavviare bash einrichten.sh e confermare nell'app sotto «Nodo»",
        "usare un server più grande",
        "usare un server più veloce (CPU attuale, NVMe)",
        "il motivo è indicato; se «non raggiungibile», aprire la porta 8080",
        "non ancora ammesso; inviare l'indirizzo di firma al gestore",
        "aprire le porte 4001/8080 presso il fornitore, controllare la memoria (docker stats)",
    ]),
    q_h="Denaro quantistico (facoltativo)",
    q="Con un tuo account <b>IBM Quantum</b>, il tuo nodo prepara la tua sezione quando viene emesso denaro quantistico; così nessuno tranne te ne conosce gli stati. Non serve hardware aggiuntivo: la misura avviene presso IBM, script e Python sullo stesso server. Il piano gratuito di IBM basta per iniziare (tempo di calcolo limitato al mese). Una volta, come root:",
    q2="Lo script chiede una volta la chiave API (non mostrata, mai nel log). Poi un timer misura automaticamente le prenotazioni, al massimo 8 al giorno (regolabile con QS_JE_TAG).",
    fuss="Completo e sempre aggiornato: aequitas.digital/network/node · docs/VALIDATOR_EINRICHTEN.md nel repository. Domande: gruppo Telegram su aequitas.digital.",
)

T["pt"] = dict(
    titel="Operar um validador Aequitas",
    unter="Guia para operadores · atualizado a 21/11/2026 · aequitas.digital/network/node",
    was_h="Do que se trata",
    was="Um validador é um computador que mantém a cadeia, verifica ele próprio cada bloco e produz blocos. Em troca, a pessoa a quem pertence recebe todos os dias uma parte igual do fundo dos validadores. Uma pessoa pode operar exatamente um validador; mais poder de cálculo não dá nenhum voto adicional.",
    brauchst_h="O que precisas",
    tab_kopf=("", "No mínimo"),
    tab=[
        ("Servidor", "servidor alugado (VPS ou servidor dedicado), Ubuntu 22.04 ou 24.04, endereço IPv4 público"),
        ("Processador", "<b>{kerne} núcleos dedicados</b> de geração atual (p. ex. AMD EPYC 9004/9005, Ryzen 7000/9000, Intel Xeon 4.ª ger.)"),
        ("Memória", "<b>{ram} GB</b>"),
        ("Disco", "<b>{platz} GB de SSD NVMe</b> livres"),
        ("Portas abertas", "8080 (API) e 4001 (ligação a outros nós)"),
        ("Desempenho", "<b>{sig} assinaturas por segundo</b>, commit da base de dados em no máximo {ms} ms"),
        ("Tu", "registado na app Aequitas (verificação facial concluída)"),
    ],
    kosten="Um servidor destes custa cerca de <b>20 a 40 euros por mês</b> na maioria dos fornecedores. Escolhe «8 vCPU dedicadas» ou um servidor dedicado, <b>não</b> a classe mais pequena com vCPU partilhadas.",
    pruef_h="Hardware fraco demais é recusado – em três pontos",
    pruef=[
        "O script de instalação verifica núcleos, memória e espaço antes de compilar.",
        "O nó mede-se a si próprio (assinaturas por segundo, base de dados). Se não chegar, o script para-o.",
        "A rede volta a medi-lo à entrada, com uma tarefa tirada de um valor aleatório. Um servidor não consegue falsear este teste.",
    ],
    nicht="<b>Nunca vão para o servidor</b> a tua carteira, a sua lista de palavras (frase-semente) nem a sua chave privada. No servidor fica apenas o <b>endereço</b> da tua carteira.",
    einr_h="Instalação: um comando, duas perguntas",
    einr="Liga-te ao servidor (Windows: PowerShell, Mac: Terminal) e cola estas linhas:",
    einr2="O script só pergunta pelo <b>endereço da tua carteira</b> (o da app, começa por 0x) e pede para <b>confirmar o endereço IP</b>. Trata de tudo o resto: palavra-passe da base de dados, configuração, compilar e arrancar (cerca de 10 minutos da primeira vez) e guardar as chaves do nó.",
    app_h="Confirmar na app",
    app=[
        "No fim o script mostra um <b>número de controlo</b> de seis algarismos.",
        "App Aequitas → separador <b>«Nó»</b>: «O teu servidor quer ligar-se a ti» com um número.",
        "Só se o número for <b>exatamente</b> igual, toca em <b>«Confirmar»</b>. As assinaturas não custam nada e não movem dinheiro.",
    ],
    app2="Sem a app (p. ex. MetaMask): abre http://SERVER-IP:8080/node-binding e assina com a mesma carteira.",
    aufn_h="Depois: admissão como produtor de blocos",
    aufn="Por agora o operador ainda admite à mão os novos produtores de blocos, até cada nó recalcular por si cada valor de um bloco. Envia-lhe o teu endereço de assinatura. Até lá o teu nó funciona como observador completo e verifica cada bloco.",
    laeuft_h="Está a funcionar? Atualizar",
    laeuft="As duas alturas devem coincidir com poucos blocos de diferença. Ctrl+C fecha o registo; o nó continua a funcionar. Para atualizar:",
    wo_h="O que fica onde",
    wo_kopf=("", "Onde"),
    wo=[
        ("Chave de assinatura, chave P2P", "só no servidor, em .env"),
        ("Endereço da tua carteira", "no servidor (é para lá que vão as recompensas)"),
        ("Chave IBM Quantum (só com dinheiro quântico)", "só no servidor, legível apenas por root"),
        ("Carteira, lista de palavras, chave privada", "<b>só contigo</b>, nunca no servidor"),
    ],
    wo2="<b>Faz cópia do ficheiro .env.</b> Se se perder, o nó recebe uma nova identidade e tem de ser ligado e admitido de novo.",
    fehler_h="Se algo correr mal",
    fehler_kopf=("Mensagem", "O que fazer"),
    fehler=_fehler([
        "primeiro registar-se na app",
        "voltar a correr bash einrichten.sh e confirmar na app em «Nó»",
        "usar um servidor maior",
        "usar um servidor mais rápido (CPU atual, NVMe)",
        "o motivo é indicado; se «inacessível», abrir a porta 8080",
        "ainda não admitido; enviar o endereço de assinatura ao operador",
        "abrir as portas 4001/8080 no fornecedor, verificar a memória (docker stats)",
    ]),
    q_h="Dinheiro quântico (opcional)",
    q="Com a tua própria conta <b>IBM Quantum</b>, o teu nó prepara a tua própria secção quando é emitido dinheiro quântico; assim ninguém além de ti conhece os seus estados. Não precisas de hardware adicional: a medição é feita na IBM, os scripts e o Python no mesmo servidor. O plano gratuito da IBM chega para começar (tempo de cálculo limitado por mês). Uma vez, como root:",
    q2="O script pede uma vez a chave de API (não é mostrada, nunca vai para o registo). Depois um temporizador mede automaticamente o que foi reservado, no máximo 8 por dia (ajustável com QS_JE_TAG).",
    fuss="Completo e sempre atual: aequitas.digital/network/node · docs/VALIDATOR_EINRICHTEN.md no repositório. Perguntas: grupo de Telegram em aequitas.digital.",
)

T["tr"] = dict(
    titel="Bir Aequitas doğrulayıcısı işletmek",
    unter="İşletmeci kılavuzu · 21.11.2026 itibarıyla · aequitas.digital/network/node",
    was_h="Ne olduğu",
    was="Doğrulayıcı, zinciri tutan, her bloğu kendisi kontrol eden ve kendisi blok üreten bir bilgisayardır. Karşılığında sahibi olan insan her gün doğrulayıcı havuzundan eşit bir pay alır. Bir insan tam olarak bir doğrulayıcı işletebilir; daha fazla işlem gücü ek oy getirmez.",
    brauchst_h="Neye ihtiyacın var",
    tab_kopf=("", "En az"),
    tab=[
        ("Sunucu", "kiralık sunucu (VPS veya kök sunucu), Ubuntu 22.04 veya 24.04, genel IPv4 adresi"),
        ("İşlemci", "güncel nesil <b>{kerne} ayrılmış çekirdek</b> (ör. AMD EPYC 9004/9005, Ryzen 7000/9000, Intel Xeon 4. nesil)"),
        ("Bellek", "<b>{ram} GB</b>"),
        ("Disk", "<b>{platz} GB boş NVMe SSD</b>"),
        ("Açık portlar", "8080 (API) ve 4001 (diğer düğümlerle bağlantı)"),
        ("Performans", "saniyede <b>{sig} imza</b>, veritabanı commit en fazla {ms} ms"),
        ("Sen", "Aequitas uygulamasında kayıtlı (yüz kontrolü tamamlanmış)"),
    ],
    kosten="Böyle bir sunucu çoğu sağlayıcıda ayda yaklaşık <b>20 ila 40 euro</b> tutar. “8 vCPU ayrılmış” veya bir kök sunucu seç; paylaşılan vCPU’lu en küçük sınıfı <b>seçme</b>.",
    pruef_h="Zayıf donanım reddedilir – üç noktada",
    pruef=[
        "Kurulum betiği derlemeden önce çekirdekleri, belleği ve alanı kontrol eder.",
        "Düğüm kendini ölçer (saniyedeki imzalar, veritabanı). Yetmezse betik onu durdurur.",
        "Ağ katılırken onu rastgele bir değerden türetilen bir görevle yeniden ölçer. Bir sunucu bu testi kandıramaz.",
    ],
    nicht="Cüzdanın, kelime listesi (seed ifadesi) ve özel anahtarı <b>asla sunucuya konmaz</b>. Sunucuda yalnızca cüzdanının <b>adresi</b> bulunur.",
    einr_h="Kurulum: bir komut, iki soru",
    einr="Sunucuya bağlan (Windows: PowerShell, Mac: Terminal) ve şu satırları yapıştır:",
    einr2="Betik yalnızca <b>cüzdan adresini</b> (uygulamadaki, 0x ile başlayan adres) sorar ve <b>IP adresini onaylamanı</b> ister. Gerisini kendisi yapar: veritabanı parolası, yapılandırma, derleme ve başlatma (ilk seferde yaklaşık 10 dakika) ve düğümün anahtarlarını kaydetme.",
    app_h="Uygulamada onayla",
    app=[
        "Betik sonunda altı haneli bir <b>kontrol sayısı</b> gösterir.",
        "Aequitas uygulaması → <b>“Düğüm”</b> sekmesi: bir sayıyla “Sunucun seninle bağlantı kurmak istiyor”.",
        "Yalnızca sayı <b>tam olarak</b> aynıysa <b>“Onayla”</b>ya dokun. İmzalar ücretsizdir ve para hareket ettirmez.",
    ],
    app2="Uygulama olmadan (ör. MetaMask): http://SERVER-IP:8080/node-binding adresini aç ve aynı cüzdanla imzala.",
    aufn_h="Sonra: blok üretici olarak kabul",
    aufn="Şimdilik işletmeci yeni blok üreticilerini elle kabul ediyor; bu, her düğüm bir bloğun her değerini kendisi yeniden hesaplayana kadar sürecek. İmzalama adresini ona gönder. O zamana kadar düğümün tam bir gözlemci olarak çalışır ve her bloğu kontrol eder.",
    laeuft_h="Çalışıyor mu? Güncelleme",
    laeuft="İki yükseklik birkaç blok farkla aynı olmalıdır. Ctrl+C günlük görünümünü kapatır; düğüm çalışmaya devam eder. Güncellemek için:",
    wo_h="Ne nerede durur",
    wo_kopf=("", "Nerede"),
    wo=[
        ("İmzalama anahtarı, P2P anahtarı", "yalnızca sunucuda, .env içinde"),
        ("Cüzdan adresin", "sunucuda (ödüller oraya gider)"),
        ("IBM Quantum anahtarı (yalnızca kuantum parayla)", "yalnızca sunucuda, yalnızca root okuyabilir"),
        ("Cüzdan, kelime listesi, özel anahtar", "<b>yalnızca sende</b>, asla sunucuda değil"),
    ],
    wo2="<b>.env dosyasını yedekle.</b> Kaybolursa düğüm yeni bir kimlik alır; yeniden bağlanması ve kabul edilmesi gerekir.",
    fehler_h="Bir şey ters giderse",
    fehler_kopf=("Mesaj", "Ne yapmalı"),
    fehler=_fehler([
        "önce uygulamada kayıt ol",
        "bash einrichten.sh’i yeniden çalıştır ve uygulamada “Düğüm” altında onayla",
        "daha büyük bir sunucu kullan",
        "daha hızlı bir sunucu kullan (güncel CPU, NVMe)",
        "neden gösterilir; “erişilemiyor” ise 8080 portunu aç",
        "henüz kabul edilmedi; imzalama adresini işletmeciye gönder",
        "sağlayıcıda 4001/8080 portlarını aç, belleği kontrol et (docker stats)",
    ]),
    q_h="Kuantum para (isteğe bağlı)",
    q="Kendi <b>IBM Quantum</b> hesabınla düğümün, kuantum para çıkarıldığında kendi bölümünü hazırlar; böylece onun durumlarını senden başka kimse bilmez. Ek donanım gerekmez: ölçüm IBM’de, betikler ve Python aynı sunucuda çalışır. Başlangıç için IBM’in ücretsiz planı yeterlidir (ayda sınırlı işlem süresi). Bir kez, root olarak:",
    q2="Betik API anahtarını bir kez sorar (gösterilmez, günlüğe asla yazılmaz). Ardından bir zamanlayıcı ayrılmış ölçümleri otomatik yapar, günde en fazla 8 (QS_JE_TAG ile ayarlanabilir).",
    fuss="Ayrıntılı ve her zaman güncel: aequitas.digital/network/node · depoda docs/VALIDATOR_EINRICHTEN.md. Sorular: aequitas.digital’deki Telegram grubu.",
)

T["id"] = dict(
    titel="Menjalankan validator Aequitas",
    unter="Panduan operator · per 21/11/2026 · aequitas.digital/network/node",
    was_h="Tentang apa",
    was="Validator adalah komputer yang menyimpan rantai, memeriksa sendiri setiap blok, dan menghasilkan blok sendiri. Sebagai gantinya, manusia pemiliknya menerima bagian yang sama dari dana validator setiap hari. Satu manusia hanya dapat menjalankan satu validator; daya komputasi lebih besar tidak memberi suara tambahan.",
    brauchst_h="Yang kamu butuhkan",
    tab_kopf=("", "Minimal"),
    tab=[
        ("Server", "server sewaan (VPS atau server dedicated), Ubuntu 22.04 atau 24.04, alamat IPv4 publik"),
        ("Prosesor", "<b>{kerne} inti dedicated</b> generasi terkini (mis. AMD EPYC 9004/9005, Ryzen 7000/9000, Intel Xeon gen. ke-4)"),
        ("Memori", "<b>{ram} GB</b>"),
        ("Disk", "<b>{platz} GB SSD NVMe</b> kosong"),
        ("Port terbuka", "8080 (API) dan 4001 (koneksi ke node lain)"),
        ("Kinerja", "<b>{sig} tanda tangan per detik</b>, commit basis data paling lama {ms} ms"),
        ("Kamu", "terdaftar di aplikasi Aequitas (pemeriksaan wajah selesai)"),
    ],
    kosten="Server seperti ini berbiaya sekitar <b>20 sampai 40 euro per bulan</b> di kebanyakan penyedia. Pilih “8 vCPU dedicated” atau server dedicated, <b>bukan</b> kelas terkecil dengan vCPU bersama.",
    pruef_h="Perangkat keras yang terlalu lemah ditolak – di tiga titik",
    pruef=[
        "Skrip pemasangan memeriksa inti, memori, dan ruang sebelum membangun.",
        "Node mengukur dirinya sendiri (tanda tangan per detik, basis data). Jika kurang, skrip menghentikannya.",
        "Jaringan mengukurnya lagi saat bergabung, dengan tugas dari sebuah nilai acak. Server tidak bisa memalsukan uji ini.",
    ],
    nicht="<b>Jangan pernah taruh di server</b> dompetmu, daftar katanya (frasa benih), atau kunci privatnya. Di server hanya ada <b>alamat</b> dompetmu.",
    einr_h="Pemasangan: satu perintah, dua pertanyaan",
    einr="Sambungkan ke server (Windows: PowerShell, Mac: Terminal) lalu tempel baris-baris ini:",
    einr2="Skrip hanya menanyakan <b>alamat dompetmu</b> (alamat dari aplikasi, diawali 0x) dan meminta <b>konfirmasi alamat IP</b>. Sisanya dikerjakan sendiri: kata sandi basis data, konfigurasi, membangun dan menjalankan (sekitar 10 menit pertama kali), serta menyimpan kunci node.",
    app_h="Konfirmasi di aplikasi",
    app=[
        "Di akhir, skrip menampilkan <b>angka kontrol</b> enam digit.",
        "Aplikasi Aequitas → tab <b>“Node”</b>: “Servermu ingin terhubung denganmu” dengan sebuah angka.",
        "Hanya jika angkanya <b>persis</b> sama, ketuk <b>“Konfirmasi”</b>. Tanda tangan ini gratis dan tidak memindahkan uang.",
    ],
    app2="Tanpa aplikasi (mis. MetaMask): buka http://SERVER-IP:8080/node-binding dan tanda tangani dengan dompet yang sama.",
    aufn_h="Setelah itu: diterima sebagai produsen blok",
    aufn="Untuk saat ini operator masih menerima produsen blok baru secara manual, sampai setiap node menghitung ulang sendiri setiap nilai sebuah blok. Kirim alamat penandatanganmu kepadanya. Sampai saat itu node-mu berjalan sebagai pengamat penuh dan memeriksa setiap blok.",
    laeuft_h="Apakah berjalan? Memperbarui",
    laeuft="Kedua tinggi seharusnya sama, selisih beberapa blok saja. Ctrl+C menutup tampilan log; node tetap berjalan. Untuk memperbarui:",
    wo_h="Apa disimpan di mana",
    wo_kopf=("", "Di mana"),
    wo=[
        ("Kunci penanda tangan, kunci P2P", "hanya di server, di .env"),
        ("Alamat dompetmu", "di server (ke sana imbalan dikirim)"),
        ("Kunci IBM Quantum (hanya dengan uang kuantum)", "hanya di server, hanya bisa dibaca root"),
        ("Dompet, daftar kata, kunci privat", "<b>hanya padamu</b>, tidak pernah di server"),
    ],
    wo2="<b>Cadangkan berkas .env.</b> Jika hilang, node mendapat identitas baru dan harus diikat serta diterima ulang.",
    fehler_h="Jika ada masalah",
    fehler_kopf=("Pesan", "Yang dilakukan"),
    fehler=_fehler([
        "daftar dulu di aplikasi",
        "jalankan lagi bash einrichten.sh dan konfirmasi di aplikasi pada “Node”",
        "gunakan server yang lebih besar",
        "gunakan server yang lebih cepat (CPU terkini, NVMe)",
        "alasannya ditampilkan; jika “tidak terjangkau”, buka port 8080",
        "belum diterima; kirim alamat penanda tangan ke operator",
        "buka port 4001/8080 di penyedia, periksa memori (docker stats)",
    ]),
    q_h="Uang kuantum (opsional)",
    q="Dengan akun <b>IBM Quantum</b> milikmu sendiri, node-mu menyiapkan bagianmu sendiri saat uang kuantum diterbitkan; tak seorang pun selain kamu mengetahui keadaannya. Tidak perlu perangkat keras tambahan: pengukuran berjalan di IBM, skrip dan Python di server yang sama. Paket gratis IBM cukup untuk memulai (waktu komputasi terbatas per bulan). Sekali, sebagai root:",
    q2="Skrip menanyakan kunci API sekali (tidak ditampilkan, tidak pernah masuk log). Setelah itu sebuah timer mengukur pesanan secara otomatis, paling banyak 8 per hari (dapat diatur dengan QS_JE_TAG).",
    fuss="Lengkap dan selalu terkini: aequitas.digital/network/node · docs/VALIDATOR_EINRICHTEN.md di repositori. Pertanyaan: grup Telegram di aequitas.digital.",
)

T["ru"] = dict(
    titel="Как запустить валидатор Aequitas",
    unter="Руководство для операторов · по состоянию на 21.11.2026 · aequitas.digital/network/node",
    was_h="О чём речь",
    was="Валидатор — это компьютер, который ведёт цепочку, сам проверяет каждый блок и сам производит блоки. За это человек, которому он принадлежит, каждый день получает равную долю из фонда валидаторов. Один человек может держать ровно один валидатор; больше вычислительной мощности не даёт дополнительного голоса.",
    brauchst_h="Что нужно",
    tab_kopf=("", "Минимум"),
    tab=[
        ("Сервер", "арендованный сервер (VPS или выделенный), Ubuntu 22.04 или 24.04, публичный IPv4-адрес"),
        ("Процессор", "<b>{kerne} выделенных ядер</b> текущего поколения (например, AMD EPYC 9004/9005, Ryzen 7000/9000, Intel Xeon 4-го пок.)"),
        ("Память", "<b>{ram} ГБ</b>"),
        ("Диск", "<b>{platz} ГБ свободно на NVMe SSD</b>"),
        ("Открытые порты", "8080 (API) и 4001 (связь с другими узлами)"),
        ("Производительность", "<b>{sig} подписей в секунду</b>, коммит базы данных не дольше {ms} мс"),
        ("Ты", "зарегистрирован в приложении Aequitas (проверка лица пройдена)"),
    ],
    kosten="Такой сервер у большинства провайдеров стоит примерно <b>20–40 евро в месяц</b>. Выбирай «8 выделенных vCPU» или выделенный сервер, <b>не</b> самый маленький тариф с общими vCPU.",
    pruef_h="Слишком слабое оборудование отклоняется — в трёх местах",
    pruef=[
        "Скрипт установки проверяет ядра, память и место до сборки.",
        "Узел измеряет сам себя (подписи в секунду, база данных). Если не хватает, скрипт его останавливает.",
        "Сеть ещё раз измеряет его при подключении задачей из случайного значения. Обмануть эту проверку сервер не может.",
    ],
    nicht="<b>Никогда не клади на сервер</b> свой кошелёк, его список слов (сид-фразу) и закрытый ключ. На сервере хранится только <b>адрес</b> кошелька.",
    einr_h="Установка: одна команда, два вопроса",
    einr="Подключись к серверу (Windows: PowerShell, Mac: Терминал) и вставь эти строки:",
    einr2="Скрипт спрашивает только <b>адрес кошелька</b> (адрес из приложения, начинается с 0x) и просит <b>подтвердить IP-адрес</b>. Всё остальное он делает сам: пароль базы данных, настройку, сборку и запуск (в первый раз около 10 минут) и сохранение ключей узла.",
    app_h="Подтверждение в приложении",
    app=[
        "В конце скрипт показывает шестизначное <b>контрольное число</b>.",
        "Приложение Aequitas → вкладка <b>«Узел»</b>: «Твой сервер хочет подключиться к тебе» с числом.",
        "Только если число <b>точно</b> совпадает, нажми <b>«Подтвердить»</b>. Подписи бесплатны и не переводят деньги.",
    ],
    app2="Без приложения (например, MetaMask): открой http://SERVER-IP:8080/node-binding и подпиши тем же кошельком.",
    aufn_h="Потом: допуск как производителя блоков",
    aufn="Пока оператор ещё вручную допускает новых производителей блоков — до тех пор, пока каждый узел сам не пересчитывает каждое значение блока. Отправь ему свой адрес подписи. До этого твой узел работает как полноценный наблюдатель и проверяет каждый блок.",
    laeuft_h="Работает? Обновление",
    laeuft="Обе высоты должны совпадать с точностью до нескольких блоков. Ctrl+C закрывает просмотр журнала; узел продолжает работать. Обновление:",
    wo_h="Что где хранится",
    wo_kopf=("", "Где"),
    wo=[
        ("Ключ подписи, P2P-ключ", "только на сервере, в .env"),
        ("Адрес твоего кошелька", "на сервере (туда идут вознаграждения)"),
        ("Ключ IBM Quantum (только с квантовыми деньгами)", "только на сервере, читать может только root"),
        ("Кошелёк, список слов, закрытый ключ", "<b>только у тебя</b>, никогда на сервере"),
    ],
    wo2="<b>Сохрани копию файла .env.</b> Если он потеряется, узел получит новую личность, и его придётся заново привязать и допустить.",
    fehler_h="Если что-то не работает",
    fehler_kopf=("Сообщение", "Что делать"),
    fehler=_fehler([
        "сначала зарегистрироваться в приложении",
        "снова запустить bash einrichten.sh и подтвердить в приложении во вкладке «Узел»",
        "взять сервер мощнее",
        "взять более быстрый сервер (современный CPU, NVMe)",
        "причина указана; если «недоступен», открыть порт 8080",
        "ещё не допущен; отправить адрес подписи оператору",
        "открыть порты 4001/8080 у провайдера, проверить память (docker stats)",
    ]),
    q_h="Квантовые деньги (по желанию)",
    q="С собственным аккаунтом <b>IBM Quantum</b> твой узел при выпуске квантовых денег готовит свой участок; тогда его состояния не знает никто, кроме тебя. Дополнительное оборудование не нужно: измерение выполняется у IBM, скрипты и Python — на том же сервере. Для начала хватает бесплатного плана IBM (ограниченное время вычислений в месяц). Один раз, от root:",
    q2="Скрипт один раз спрашивает API-ключ (не показывается и никогда не попадает в журнал). Затем таймер автоматически выполняет заказанные измерения, не больше 8 в день (настраивается через QS_JE_TAG).",
    fuss="Подробно и всегда актуально: aequitas.digital/network/node · docs/VALIDATOR_EINRICHTEN.md в репозитории. Вопросы: группа в Telegram на aequitas.digital.",
)

T["zh"] = dict(
    titel="运行 Aequitas 验证者节点",
    unter="运营者指南 · 截至 2026 年 11 月 21 日 · aequitas.digital/network/node",
    was_h="这是什么",
    was="验证者是一台保存整条链、自行检查每个区块并自行产生区块的计算机。作为回报，拥有它的人每天从验证者奖池中获得同等的一份。一个人只能运行一个验证者；更多算力不会带来额外的投票权。",
    brauchst_h="你需要什么",
    tab_kopf=("", "最低要求"),
    tab=[
        ("服务器", "租用的服务器（VPS 或独立服务器），Ubuntu 22.04 或 24.04，公网 IPv4 地址"),
        ("处理器", "当前一代的 <b>{kerne} 个独享核心</b>（如 AMD EPYC 9004/9005、Ryzen 7000/9000、Intel Xeon 第 4 代）"),
        ("内存", "<b>{ram} GB</b>"),
        ("硬盘", "<b>{platz} GB 可用 NVMe 固态硬盘</b>"),
        ("开放端口", "8080（接口）和 4001（与其他节点连接）"),
        ("性能", "<b>每秒 {sig} 次签名</b>，数据库提交不超过 {ms} 毫秒"),
        ("你本人", "已在 Aequitas 应用中注册（已完成人脸检查）"),
    ],
    kosten="在大多数服务商处，这样的服务器每月约 <b>20 至 40 欧元</b>。请选择“8 个独享 vCPU”或独立服务器，<b>不要</b>选共享 vCPU 的最小规格。",
    pruef_h="硬件太弱会被拒绝——共有三道关",
    pruef=[
        "安装脚本在构建前检查核心数、内存和空间。",
        "节点会自测（每秒签名数、数据库）。达不到要求时，脚本会把它停下。",
        "加入网络时，网络会再用一个由随机值生成的计算任务测量它。服务器无法在这项测试中作假。",
    ],
    nicht="<b>绝不要放到服务器上的</b>：你的钱包、助记词和私钥。服务器上只有你钱包的<b>地址</b>。",
    einr_h="安装：一条命令，两个问题",
    einr="连接到服务器（Windows：PowerShell；Mac：终端），然后粘贴以下几行：",
    einr2="脚本只会询问<b>你的钱包地址</b>（应用中以 0x 开头的地址），并请你<b>确认 IP 地址</b>。其余一切由它自动完成：数据库密码、配置、构建和启动（第一次约 10 分钟），以及保存节点的密钥。",
    app_h="在应用中确认",
    app=[
        "最后，脚本会显示一个六位数的<b>核对数字</b>。",
        "Aequitas 应用 → <b>“节点”</b>标签：出现带有数字的“你的服务器想与你连接”。",
        "只有当数字<b>完全</b>一致时，才点<b>“确认”</b>。这些签名免费，也不会转移任何资金。",
    ],
    app2="不使用应用（例如 MetaMask）：打开 http://SERVER-IP:8080/node-binding，用同一个钱包签名。",
    aufn_h="之后：被接纳为区块生产者",
    aufn="目前，运营者仍手动接纳新的区块生产者，直到每个节点都自行重新计算区块中的每个值为止。请把你的签名地址发给运营者。在此之前，你的节点作为完整的观察者运行，并检查每个区块。",
    laeuft_h="在运行吗？如何更新",
    laeuft="两个高度应相差不超过几个区块。按 Ctrl+C 只会关闭日志显示，节点继续运行。更新：",
    wo_h="什么放在哪里",
    wo_kopf=("", "位置"),
    wo=[
        ("签名密钥、P2P 密钥", "只在服务器上，位于 .env"),
        ("你的钱包地址", "在服务器上（奖励发往这里）"),
        ("IBM Quantum 密钥（仅限量子货币）", "只在服务器上，只有 root 可读"),
        ("钱包、助记词、私钥", "<b>只在你自己手中</b>，绝不放在服务器上"),
    ],
    wo2="<b>请备份 .env 文件。</b>一旦丢失，节点会得到新的身份，必须重新绑定并重新被接纳。",
    fehler_h="出现问题时",
    fehler_kopf=("提示信息", "怎么办"),
    fehler=_fehler([
        "先在应用中注册",
        "重新运行 bash einrichten.sh，并在应用的“节点”中确认",
        "换一台更大的服务器",
        "换一台更快的服务器（新一代 CPU、NVMe）",
        "原因会一并显示；若为“无法访问”，请开放 8080 端口",
        "尚未被接纳；把签名地址发给运营者",
        "在服务商处开放 4001/8080 端口，检查内存（docker stats）",
    ]),
    q_h="量子货币（可选）",
    q="有了你自己的 <b>IBM Quantum</b> 账户，在发行量子货币时你的节点会制备你自己的那一段；这样除你之外没有人知道它的状态。不需要额外硬件：测量在 IBM 上运行，脚本和 Python 在同一台服务器上。起步时 IBM 的免费方案就够用（每月计算时间有限）。只需以 root 身份执行一次：",
    q2="脚本只询问一次 API 密钥（输入时不显示，也绝不写入日志）。之后由定时器自动完成预约的测量，每天最多 8 次（可用 QS_JE_TAG 调整）。",
    fuss="完整且始终最新：aequitas.digital/network/node · 代码库中的 docs/VALIDATOR_EINRICHTEN.md。有问题请到 aequitas.digital 上的 Telegram 群组。",
)

T["ar"] = dict(
    titel="تشغيل مدقق Aequitas",
    unter="دليل المشغّل · بحسب حالة 21 نوفمبر 2026 · aequitas.digital/network/node",
    was_h="ما هو",
    was="المدقق حاسوب يحفظ السلسلة ويتحقق من كل كتلة بنفسه وينتج الكتل بنفسه. وفي المقابل يحصل الإنسان الذي يملكه كل يوم على حصة متساوية من صندوق المدققين. يستطيع الإنسان تشغيل مدقق واحد فقط؛ ولا تمنح قدرة حوسبة أكبر أي صوت إضافي.",
    brauchst_h="ما تحتاجه",
    tab_kopf=("", "الحد الأدنى"),
    tab=[
        ("الخادم", "خادم مستأجر (VPS أو خادم مخصص)، Ubuntu 22.04 أو 24.04، عنوان IPv4 عام"),
        ("المعالج", "<b>{kerne} أنوية مخصصة</b> من جيل حديث (مثل AMD EPYC 9004/9005 أو Ryzen 7000/9000 أو Intel Xeon الجيل الرابع)"),
        ("الذاكرة", "<b>{ram} GB</b>"),
        ("القرص", "<b>{platz} GB</b> فارغة على قرص <b>NVMe SSD</b>"),
        ("المنافذ المفتوحة", "8080 (الواجهة) و4001 (الاتصال بالعُقد الأخرى)"),
        ("الأداء", "<b>{sig} توقيع في الثانية</b>، وحفظ قاعدة البيانات في {ms} ms على الأكثر"),
        ("أنت", "مسجّل في تطبيق Aequitas (اكتمل فحص الوجه)"),
    ],
    kosten="تبلغ كلفة مثل هذا الخادم لدى معظم المزوّدين نحو <b>20 إلى 40 يورو شهريًا</b>. اختر «8 vCPU مخصصة» أو خادمًا مخصصًا، و<b>ليس</b> أصغر فئة بأنوية مشتركة.",
    pruef_h="يُرفض العتاد الضعيف — في ثلاثة مواضع",
    pruef=[
        "يفحص سكربت الإعداد الأنوية والذاكرة والمساحة قبل البناء.",
        "تقيس العقدة نفسها (التوقيعات في الثانية، قاعدة البيانات). وإن لم تكفِ يوقفها السكربت.",
        "تقيسها الشبكة مرة أخرى عند الانضمام بمهمة مشتقة من قيمة عشوائية. ولا يستطيع الخادم تزييف هذا الاختبار.",
    ],
    nicht="<b>لا تضع أبدًا على الخادم</b> محفظتك ولا قائمة كلماتها (عبارة الاسترداد) ولا مفتاحها الخاص. على الخادم يوجد <b>عنوان</b> محفظتك فقط.",
    einr_h="الإعداد: أمر واحد وسؤالان",
    einr="اتصل بالخادم (Windows: PowerShell، Mac: Terminal) والصق هذه الأسطر:",
    einr2="يسأل السكربت فقط عن <b>عنوان محفظتك</b> (العنوان من التطبيق، ويبدأ بـ 0x) ويطلب <b>تأكيد عنوان IP</b>. ويتولى كل ما عدا ذلك بنفسه: كلمة مرور قاعدة البيانات، والإعداد، والبناء والتشغيل (نحو 10 دقائق في المرة الأولى)، وحفظ مفاتيح العقدة.",
    app_h="التأكيد في التطبيق",
    app=[
        "في النهاية يعرض السكربت <b>رقم تحقق</b> من ستة أرقام.",
        "تطبيق Aequitas ← تبويب <b>«العقدة»</b>: «خادمك يريد الاتصال بك» مع رقم.",
        "فقط إذا كان الرقم مطابقًا <b>تمامًا</b> اضغط <b>«تأكيد»</b>. التوقيعات مجانية ولا تنقل أي مال.",
    ],
    app2="من دون التطبيق (مثل MetaMask): افتح http://SERVER-IP:8080/node-binding ووقّع بالمحفظة نفسها.",
    aufn_h="بعد ذلك: القبول كمنتج للكتل",
    aufn="حاليًا ما زال المشغّل يقبل منتجي الكتل الجدد يدويًا، إلى أن تعيد كل عقدة حساب كل قيمة في الكتلة بنفسها. أرسل إليه عنوان التوقيع الخاص بك. وحتى ذلك الحين تعمل عقدتك مراقبًا كاملًا وتتحقق من كل كتلة.",
    laeuft_h="هل تعمل؟ التحديث",
    laeuft="يجب أن يتطابق الارتفاعان بفارق بضع كتل فقط. يغلق Ctrl+C عرض السجل فقط، وتستمر العقدة في العمل. للتحديث:",
    wo_h="ما الذي يوجد أين",
    wo_kopf=("", "المكان"),
    wo=[
        ("مفتاح التوقيع، مفتاح P2P", "على الخادم فقط، في .env"),
        ("عنوان محفظتك", "على الخادم (تذهب المكافآت إليه)"),
        ("مفتاح IBM Quantum (مع المال الكمومي فقط)", "على الخادم فقط، ولا يقرؤه إلا root"),
        ("المحفظة وقائمة الكلمات والمفتاح الخاص", "<b>معك فقط</b>، ولا تكون على الخادم أبدًا"),
    ],
    wo2="<b>احتفظ بنسخة من ملف .env.</b> إذا ضاع تحصل العقدة على هوية جديدة ويجب ربطها وقبولها من جديد.",
    fehler_h="إذا حدث خطأ",
    fehler_kopf=("الرسالة", "ما العمل"),
    fehler=_fehler([
        "سجّل أولًا في التطبيق",
        "شغّل bash einrichten.sh من جديد وأكّد في التطبيق تحت «العقدة»",
        "استخدم خادمًا أكبر",
        "استخدم خادمًا أسرع (معالج حديث، NVMe)",
        "يُذكر السبب؛ وإن كان «غير قابل للوصول» فافتح المنفذ 8080",
        "لم تُقبل بعد؛ أرسل عنوان التوقيع إلى المشغّل",
        "افتح المنفذين 4001/8080 لدى المزوّد وافحص الذاكرة (docker stats)",
    ]),
    q_h="المال الكمومي (اختياري)",
    q="بحسابك الخاص في <b>IBM Quantum</b> تحضّر عقدتك قسمك الخاص عند إصدار المال الكمومي؛ فلا يعرف حالاته أحد سواك. لا تحتاج إلى عتاد إضافي: يجري القياس لدى IBM، وتعمل السكربتات وPython على الخادم نفسه. تكفي خطة IBM المجانية للبداية (وقت حوسبة محدود شهريًا). مرة واحدة بصلاحيات root:",
    q2="يسأل السكربت مرة واحدة عن مفتاح API (لا يظهر ولا يُكتب في السجل أبدًا). بعد ذلك يقيس مؤقّت الحجوزات تلقائيًا، بحد أقصى 8 في اليوم (قابل للتعديل عبر QS_JE_TAG).",
    fuss="بالتفصيل ومحدّث دائمًا: aequitas.digital/network/node · ملف docs/VALIDATOR_EINRICHTEN.md في المستودع. للأسئلة: مجموعة Telegram على aequitas.digital.",
)

T["hi"] = dict(
    titel="Aequitas वैलिडेटर चलाना",
    unter="संचालक गाइड · 21.11.2026 की स्थिति · aequitas.digital/network/node",
    was_h="यह क्या है",
    was="वैलिडेटर एक कंप्यूटर है जो चेन रखता है, हर ब्लॉक को ख़ुद जाँचता है और ख़ुद ब्लॉक बनाता है। बदले में, जिस इंसान का यह है उसे हर दिन वैलिडेटर कोष से बराबर हिस्सा मिलता है। एक इंसान ठीक एक वैलिडेटर चला सकता है; ज़्यादा कंप्यूटिंग शक्ति से कोई अतिरिक्त वोट नहीं मिलता।",
    brauchst_h="तुम्हें क्या चाहिए",
    tab_kopf=("", "कम से कम"),
    tab=[
        ("सर्वर", "किराये का सर्वर (VPS या रूट सर्वर), Ubuntu 22.04 या 24.04, सार्वजनिक IPv4 पता"),
        ("प्रोसेसर", "मौजूदा पीढ़ी के <b>{kerne} समर्पित कोर</b> (जैसे AMD EPYC 9004/9005, Ryzen 7000/9000, Intel Xeon चौथी पीढ़ी)"),
        ("मेमोरी", "<b>{ram} GB</b>"),
        ("डिस्क", "<b>{platz} GB खाली NVMe SSD</b>"),
        ("खुले पोर्ट", "8080 (API) और 4001 (दूसरे नोड से जुड़ाव)"),
        ("प्रदर्शन", "<b>प्रति सेकंड {sig} हस्ताक्षर</b>, डेटाबेस कमिट अधिकतम {ms} ms"),
        ("तुम ख़ुद", "Aequitas ऐप में पंजीकृत (चेहरे की जाँच पूरी)"),
    ],
    kosten="ऐसा सर्वर ज़्यादातर प्रदाताओं के यहाँ लगभग <b>20 से 40 यूरो प्रति माह</b> का होता है। “8 vCPU समर्पित” या रूट सर्वर चुनो, साझा vCPU वाली सबसे छोटी श्रेणी <b>नहीं</b>।",
    pruef_h="कमज़ोर हार्डवेयर अस्वीकार होता है – तीन जगहों पर",
    pruef=[
        "सेटअप स्क्रिप्ट बनाने से पहले कोर, मेमोरी और जगह जाँचती है।",
        "नोड ख़ुद को मापता है (प्रति सेकंड हस्ताक्षर, डेटाबेस)। कम पड़ने पर स्क्रिप्ट उसे रोक देती है।",
        "जुड़ते समय नेटवर्क एक यादृच्छिक मान से बने काम से उसे फिर मापता है। सर्वर इस जाँच में धोखा नहीं दे सकता।",
    ],
    nicht="<b>सर्वर पर कभी न रखें</b>: तुम्हारा वॉलेट, उसकी शब्द-सूची (सीड फ़्रेज़) और उसकी निजी कुंजी। सर्वर पर केवल तुम्हारे वॉलेट का <b>पता</b> रहता है।",
    einr_h="सेटअप: एक कमांड, दो सवाल",
    einr="सर्वर से जुड़ो (Windows: PowerShell, Mac: Terminal) और ये पंक्तियाँ चिपकाओ:",
    einr2="स्क्रिप्ट केवल <b>तुम्हारे वॉलेट का पता</b> (ऐप वाला पता, 0x से शुरू) पूछती है और <b>IP पते की पुष्टि</b> माँगती है। बाक़ी सब वह ख़ुद करती है: डेटाबेस पासवर्ड, कॉन्फ़िगरेशन, बनाना और चलाना (पहली बार लगभग 10 मिनट) और नोड की कुंजियाँ सहेजना।",
    app_h="ऐप में पुष्टि करो",
    app=[
        "अंत में स्क्रिप्ट छह अंकों की एक <b>जाँच संख्या</b> दिखाती है।",
        "Aequitas ऐप → <b>“नोड”</b> टैब: एक संख्या के साथ “तुम्हारा सर्वर तुमसे जुड़ना चाहता है”।",
        "केवल तभी <b>“पुष्टि करें”</b> दबाओ जब संख्या <b>बिल्कुल</b> एक जैसी हो। हस्ताक्षर मुफ़्त हैं और कोई पैसा नहीं हिलाते।",
    ],
    app2="ऐप के बिना (जैसे MetaMask): http://SERVER-IP:8080/node-binding खोलो और उसी वॉलेट से हस्ताक्षर करो।",
    aufn_h="इसके बाद: ब्लॉक निर्माता के रूप में शामिल होना",
    aufn="अभी संचालक नए ब्लॉक निर्माताओं को हाथ से शामिल करता है, जब तक हर नोड ब्लॉक के हर मान को ख़ुद दोबारा न गिनने लगे। उसे अपना हस्ताक्षर-पता भेजो। तब तक तुम्हारा नोड पूर्ण पर्यवेक्षक के रूप में चलता है और हर ब्लॉक जाँचता है।",
    laeuft_h="चल रहा है? अपडेट करना",
    laeuft="दोनों ऊँचाइयाँ कुछ ब्लॉक के अंतर तक बराबर होनी चाहिए। Ctrl+C केवल लॉग बंद करता है; नोड चलता रहता है। अपडेट के लिए:",
    wo_h="क्या कहाँ रहता है",
    wo_kopf=("", "कहाँ"),
    wo=[
        ("हस्ताक्षर कुंजी, P2P कुंजी", "केवल सर्वर पर, .env में"),
        ("तुम्हारे वॉलेट का पता", "सर्वर पर (पुरस्कार वहीं जाते हैं)"),
        ("IBM Quantum कुंजी (केवल क्वांटम धन के साथ)", "केवल सर्वर पर, केवल root पढ़ सकता है"),
        ("वॉलेट, शब्द-सूची, निजी कुंजी", "<b>केवल तुम्हारे पास</b>, सर्वर पर कभी नहीं"),
    ],
    wo2="<b>.env फ़ाइल की प्रति सुरक्षित रखो।</b> खो जाने पर नोड को नई पहचान मिलती है और उसे फिर से जोड़ना व शामिल करवाना पड़ता है।",
    fehler_h="अगर कुछ ग़लत हो",
    fehler_kopf=("संदेश", "क्या करें"),
    fehler=_fehler([
        "पहले ऐप में पंजीकरण करो",
        "bash einrichten.sh फिर चलाओ और ऐप में “नोड” के नीचे पुष्टि करो",
        "बड़ा सर्वर लो",
        "तेज़ सर्वर लो (नया CPU, NVMe)",
        "कारण साथ में दिखता है; “पहुँच से बाहर” होने पर पोर्ट 8080 खोलो",
        "अभी शामिल नहीं; हस्ताक्षर-पता संचालक को भेजो",
        "प्रदाता के यहाँ पोर्ट 4001/8080 खोलो, मेमोरी जाँचो (docker stats)",
    ]),
    q_h="क्वांटम धन (वैकल्पिक)",
    q="अपने ख़ुद के <b>IBM Quantum</b> खाते से, क्वांटम धन जारी होने पर तुम्हारा नोड तुम्हारा अपना हिस्सा तैयार करता है; तब उसकी अवस्थाएँ तुम्हारे सिवा कोई नहीं जानता। अतिरिक्त हार्डवेयर की ज़रूरत नहीं: माप IBM पर होता है, स्क्रिप्ट और Python उसी सर्वर पर। शुरुआत के लिए IBM का मुफ़्त प्लान काफ़ी है (प्रति माह सीमित गणना-समय)। एक बार, root के रूप में:",
    q2="स्क्रिप्ट API कुंजी एक बार पूछती है (दिखती नहीं, लॉग में कभी नहीं)। इसके बाद एक टाइमर आरक्षित माप अपने-आप करता है, दिन में अधिकतम 8 (QS_JE_TAG से बदला जा सकता है)।",
    fuss="विस्तार से और हमेशा अद्यतन: aequitas.digital/network/node · रिपॉज़िटरी में docs/VALIDATOR_EINRICHTEN.md। सवाल: aequitas.digital पर Telegram समूह।",
)

SPRACHEN = ["en", "de", "es", "fr", "it", "pt", "tr", "id", "ru", "zh", "ar", "hi"]
SCHRIFT = {
    "zh": "'Noto Sans CJK SC', 'Noto Sans', sans-serif",
    "ar": "'Noto Sans Arabic', 'Noto Sans', sans-serif",
    "hi": "'Noto Sans Devanagari', 'Noto Sans', sans-serif",
}

CSS = """
@page { size: A4; margin: 16mm 15mm 16mm 15mm; }
* { box-sizing: border-box; }
body { font-family: %(font)s; color: #1E1B4B; font-size: 9.6pt; line-height: 1.5; margin: 0; }
h1 { color: #5B21B6; font-size: 20pt; margin: 0 0 2pt; text-align: center; }
.unter { color: #6B7280; font-size: 8.6pt; text-align: center; margin-bottom: 10pt; }
h2 { color: #5B21B6; font-size: 12pt; margin: 13pt 0 4pt; padding-bottom: 2pt; border-bottom: 1px solid #DDD6FE; break-after: avoid; }
p { margin: 3pt 0; }
table { width: 100%%; border-collapse: collapse; margin: 4pt 0; break-inside: avoid; }
th { background: #EDE9FE; color: #5B21B6; text-align: start; padding: 4pt 6pt; font-size: 8.8pt; }
td { border-top: 1px solid #E5E7EB; padding: 4pt 6pt; vertical-align: top; }
td.k { width: 30%%; font-weight: 600; }
tr:nth-child(even) td { background: #F9FAFB; }
pre { direction: ltr; text-align: left; unicode-bidi: isolate; background: #1E1B4B; color: #F5F3FF; font-family: 'DejaVu Sans Mono', monospace; font-size: 8.2pt; line-height: 1.45; padding: 7pt 9pt; border-radius: 5pt; white-space: pre-wrap; word-break: break-all; margin: 4pt 0; break-inside: avoid; }
.box { background: #FFFBEB; border-inline-start: 3pt solid #B45309; padding: 5pt 8pt; margin: 5pt 0; break-inside: avoid; }
.warn { background: #FEF2F2; border-inline-start: 3pt solid #B91C1C; padding: 5pt 8pt; margin: 5pt 0; break-inside: avoid; }
.q { background: #F0FDFA; border-inline-start: 3pt solid #0F766E; padding: 5pt 8pt; margin: 5pt 0; }
ol, ul { margin: 3pt 0; padding-inline-start: 16pt; }
li { margin: 1.5pt 0; }
.fuss { color: #6B7280; font-size: 8.2pt; margin-top: 12pt; border-top: 1px solid #E5E7EB; padding-top: 5pt; }
code, .c { font-family: 'DejaVu Sans Mono', monospace; font-size: 8.4pt; direction: ltr; unicode-bidi: isolate; }
"""


# Tausendertrennzeichen je Sprache (50.000 / 50,000 / 50 000).
TRENNER = {"en": ",", "zh": ",", "hi": ",", "ar": ",", "fr": "\u202f", "ru": "\u00a0"}


def _ersetzen(s, w, lang):
    for k, v in w.items():
        s = s.replace("{" + k + "}", f"{v:,}".replace(",", TRENNER.get(lang, ".")) if k == "sig" else str(v))
    return s


def _tabelle(kopf, zeilen, w, lang, code_links=False):
    out = ["<table><tr>" + "".join(f"<th>{k}</th>" for k in kopf) + "</tr>"]
    for a, b in zeilen:
        a = f'<span class="c">{html.escape(a)}</span>' if code_links else a
        out.append(f'<tr><td class="k">{a}</td><td>{_ersetzen(b, w, lang)}</td></tr>')
    return "".join(out) + "</table>"


def seite(lang, w):
    L = T[lang]
    rtl = lang == "ar"
    css = CSS % {"font": SCHRIFT.get(lang, "'Noto Sans', 'DejaVu Sans', sans-serif")}
    def pre(t):
        return f"<pre>{html.escape(t)}</pre>"
    teile = [
        f"<h1>{L['titel']}</h1><div class='unter'>{L['unter']}</div>",
        f"<h2>{L['was_h']}</h2><p>{L['was']}</p>",
        f"<h2>{L['brauchst_h']}</h2>", _tabelle(L["tab_kopf"], L["tab"], w, lang),
        f"<div class='box'>{L['kosten']}</div>",
        f"<h2>{L['pruef_h']}</h2><ol>" + "".join(f"<li>{x}</li>" for x in L["pruef"]) + "</ol>",
        f"<div class='warn'>{L['nicht']}</div>",
        f"<h2>{L['einr_h']}</h2><p>{L['einr']}</p>", pre(BEFEHLE_EINRICHTEN), f"<p>{L['einr2']}</p>",
        f"<h2>{L['app_h']}</h2><ol>" + "".join(f"<li>{x}</li>" for x in L["app"]) + "</ol>",
        f"<p>{html.escape(L['app2'])}</p>",
        f"<h2>{L['aufn_h']}</h2><p>{L['aufn']}</p>",
        f"<h2>{L['laeuft_h']}</h2>", pre(BEFEHLE_LAEUFT), f"<p>{L['laeuft']}</p>", pre(BEFEHL_AKTUALISIEREN),
        f"<h2>{L['wo_h']}</h2>", _tabelle(L["wo_kopf"], L["wo"], w, lang),
        f"<div class='warn'>{L['wo2']}</div>",
        f"<h2>{L['fehler_h']}</h2>", _tabelle(L["fehler_kopf"], L["fehler"], w, lang, code_links=True),
        f"<h2>{L['q_h']}</h2><div class='q'><p>{L['q']}</p>" + pre(BEFEHLE_QUANTEN) + f"<p>{L['q2']}</p></div>",
        f"<div class='fuss'>{L['fuss']}</div>",
    ]
    return (f"<!doctype html><html lang='{lang}' dir='{'rtl' if rtl else 'ltr'}'><head><meta charset='utf-8'>"
            f"<title>{html.escape(L['titel'])}</title><style>{css}</style></head><body>" + "".join(teile) + "</body></html>")


def pruefen():
    """Jede Sprache hat dieselben Schluessel und Zeilenzahlen wie Deutsch."""
    de = T["de"]
    for lang in SPRACHEN:
        L = T[lang]
        if set(L) != set(de):
            sys.exit(f"{lang}: Schluessel weichen ab: {set(L) ^ set(de)}")
        for k in ("tab", "pruef", "app", "wo", "fehler"):
            if len(L[k]) != len(de[k]):
                sys.exit(f"{lang}.{k}: {len(L[k])} statt {len(de[k])} Zeilen")
        tab = " ".join(b for _, b in L["tab"])
        for ph in ("{kerne}", "{ram}", "{platz}", "{sig}", "{ms}"):
            if ph not in tab:
                sys.exit(f"{lang}: {ph} fehlt in der Tabelle")


if __name__ == "__main__":
    pruefen()
    w = mindestwerte()
    os.makedirs(AUS, exist_ok=True)
    for lang in SPRACHEN:
        with open(os.path.join(AUS, f"{lang}.html"), "w", encoding="utf-8") as f:
            f.write(seite(lang, w))
    print(f"{len(SPRACHEN)} Sprachen nach {AUS} (Mindestwerte aus einrichten.sh: {w})")
