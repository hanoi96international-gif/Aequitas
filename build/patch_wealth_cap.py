#!/usr/bin/env python3
# Updates wealth cap descriptions across all 7 languages to reflect the
# bootstrap sliding multiplier: max(5, min(N, 25)) x average AEQ balance
import sys

TARGET = r'C:\Users\aequitas-chain\x\humanity\keeper\api_html.go'

with open(TARGET, 'rb') as f:
    content = f.read().decode('utf-8')

original = content

REPLACEMENTS = [

# ─── Hardcoded HTML defaults ─────────────────────────────────────────────────

(
'Wallets exceeding 25× average balance have the excess confiscated instantly. 20% flows to UBI immediately.',
'Wallets exceeding max(5,min(N,25))× average have excess confiscated instantly. 20% flows to UBI.'
),

(
'Phase boundaries define network growth milestones. The wealth cap multiplier is currently fixed at 25× (matching the live Go code constant <em>wealthCapMultiplier = 25.0</em>) — phase-based automatic tightening is a planned future protocol upgrade.',
'Phase boundaries define network growth milestones. During bootstrap (&lt;25 registered humans) the cap uses a sliding multiplier: max(5,min(N,25))× average — 5× with 1–4 humans, grows 1× per human, reaches full 25× at 25+ humans. Prevents early-adopter whale accumulation.'
),

(
'Bootstrap · &lt;100 humans · Wealth Cap: 25× average balance · Currently active',
'Bootstrap · &lt;100 humans · Wealth Cap: max(5,min(N,25))× avg · Sliding 5×→25× until 25 humans · Currently active'
),

(
'The <strong>Wealth Cap</strong> is currently set to <strong>25× the average AEQ balance</strong> across all verified humans. This is a fixed constant in the live Go protocol code. The cap is always relative to the live average balance — so it automatically scales as the network grows without needing manual adjustment.',
'During bootstrap (&lt;25 humans): <strong>Cap = max(5, min(N, 25)) × average AEQ balance</strong>. With 1–4 humans: 5× avg = 5,000 AEQ. Grows 1× per new human. At 25+ humans: full <strong>25× avg = 25,000 AEQ permanently</strong>. The cap is always relative to the live average — scales automatically as the network grows.'
),

(
'Cap = 25× current average AEQ balance of all verified humans<br>Automatically adjusts as the network grows and balances change<br>Applies to ALL addresses except the 4 protocol pool addresses<br>Excess AEQ is instantly redistributed to the 4 redistribution pools<br>No manual intervention required — enforced at the protocol level on every incoming transfer',
'Bootstrap cap: max(5,min(N,25))× average AEQ balance<br>1–4 humans: 5× avg (5,000 AEQ) · Grows 1× per human · 25+ humans: full 25× (25,000 AEQ)<br>Applies to ALL addresses except the 4 protocol pool addresses<br>Excess AEQ instantly redistributed · Enforced at protocol level on every transfer'
),

# ─── English (JS translation keys) ───────────────────────────────────────────

(
"'phases-desc':'Phase boundaries define network growth milestones. The wealth cap multiplier is currently fixed at 25× (matching the live Go code constant wealthCapMultiplier = 25.0) — phase-based automatic tightening is a planned future protocol upgrade.',",
"'phases-desc':'Phase boundaries define network growth milestones. During bootstrap (&lt;25 registered humans) the cap uses a sliding multiplier: max(5,min(N,25))× average — 5× with 1–4 humans, grows 1× per human, reaches full 25× at 25+ humans. Prevents early-adopter accumulation before meaningful participation exists.',"
),

(
"'p0':'Bootstrap · &lt;100 humans · Wealth Cap: 25× average balance · Currently active',",
"'p0':'Bootstrap · &lt;100 humans · Wealth Cap: max(5,min(N,25))× avg · Sliding 5×→25× until 25 humans · Currently active',"
),

(
"'wealth-cap-explain':'The Wealth Cap is currently set to 25× the average AEQ balance across all verified humans. This is a fixed constant in the live Go protocol code. The cap is always relative to the live average balance — so it automatically scales as the network grows.',",
"'wealth-cap-explain':'During bootstrap (&lt;25 humans): Cap = max(5, min(N, 25)) × average AEQ balance, where N = registered humans. With 1–4 humans: 5× avg = 5,000 AEQ. Grows 1× per new human. At 25+ humans: full 25× avg = 25,000 AEQ permanently. Prevents early-adopter concentration before real participation exists. Always relative to the live average.',"
),

(
"'cap-title':'4. WEALTH CAP — Mathematical Fairness','cap-box':'Cap = 25× current average balance of all verified humans<br>Automatically adjusts as the network grows<br>Excess AEQ instantly redistributed to redistribution pools',",
"'cap-title':'4. WEALTH CAP — Mathematical Fairness','cap-box':'Bootstrap: max(5,min(N,25))× average AEQ balance<br>1–4 humans: 5× (5,000 AEQ) · Grows 1× per human · 25+: full 25× (25,000 AEQ)<br>Applies to ALL addresses except the 4 protocol pool addresses<br>Excess AEQ instantly redistributed to redistribution pools',"
),

# ─── German ──────────────────────────────────────────────────────────────────

(
"'phases-desc':'Phasengrenzen definieren Netzwerk-Wachstums-Meilensteine. Der Multiplikator ist derzeit fest auf 25× eingestellt (entspricht der Go-Code-Konstante wealthCapMultiplier = 25.0) — phasenbasierte Anpassung ist für ein zukünftiges Protokoll-Upgrade geplant.',",
"'phases-desc':'Phasengrenzen definieren Netzwerk-Wachstums-Meilensteine. Während des Bootstraps (&lt;25 registrierte Menschen) verwendet die Vermögensobergrenze einen gleitenden Multiplikator: max(5,min(N,25))× Durchschnitt — 5× bei 1–4 Menschen, wächst um 1× je Mensch, erreicht 25× ab 25+ Menschen. Verhindert Whale-Akkumulation früher Teilnehmer.',"
),

(
"'p0':'Bootstrap · &lt;100 Menschen · Vermögensobergrenze: 25× Durchschnittsguthaben · Derzeit aktiv',",
"'p0':'Bootstrap · &lt;100 Menschen · Vermögensobergrenze: max(5,min(N,25))× Durchschnitt · Gleitend 5×→25× bis 25 Menschen · Derzeit aktiv',"
),

(
"'wealth-cap-explain':'Die Vermögensobergrenze ist derzeit auf 25× des Durchschnittsguthabens aller verifizierten Menschen festgelegt. Dies ist eine feste Konstante im Live-Go-Code. Da der Wert immer relativ zum Live-Durchschnitt gilt, skaliert die Obergrenze automatisch mit dem Netzwerkwachstum.',",
"'wealth-cap-explain':'Die Vermögensobergrenze skaliert während des Bootstraps: max(5, min(N, 25)) × Durchschnittsguthaben, wobei N = Anzahl registrierter Menschen. Bei 1–4 Menschen: 5× (5.000 AEQ). Wächst um 1× je neuem Menschen. Ab 25 Menschen: dauerhaft 25× (25.000 AEQ). Verhindert frühe Konzentration vor echter Beteiligung. Immer relativ zum Live-Durchschnitt.',"
),

(
"'cap-title':'4. VERMÖGENSOBERGRENZE — Mathematische Fairness-Durchsetzung','cap-box':'Obergrenze = 25× aktuelles Durchschnittsguthaben aller verifizierten Menschen<br>Passt sich automatisch an während das Netzwerk wächst und sich Guthaben ändern<br>Gilt für ALLE Adressen außer den 4 Protokoll-Pool-Adressen<br>Überschuss-AEQ wird sofort an die 4 Umverteilungspools weitergeleitet<br>Keine manuelle Eingriffe erforderlich — auf Protokollebene bei jeder eingehenden Überweisung erzwungen',",
"'cap-title':'4. VERMÖGENSOBERGRENZE — Mathematische Fairness-Durchsetzung','cap-box':'Bootstrap: max(5,min(N,25))× Durchschnittsguthaben<br>1–4 Menschen: 5× (5.000 AEQ) · Wächst 1× je Mensch · 25+: 25× (25.000 AEQ) dauerhaft<br>Gilt für ALLE Adressen außer den 4 Protokoll-Pool-Adressen<br>Überschuss-AEQ sofort an die 4 Umverteilungspools weitergeleitet',"
),

(
"'ubi-src-cap':'Vermögensobergrenze-Überschuss','ubi-src-cap-d':'Wallets die 25× den Durchschnittssaldo überschreiten werden sofort gekappt. 20% fließt direkt an UBI.',",
"'ubi-src-cap':'Vermögensobergrenze-Überschuss','ubi-src-cap-d':'Wallets die max(5,min(N,25))× den Durchschnittssaldo überschreiten werden sofort gekappt. 20% fließt an UBI.',"
),

# ─── Spanish ─────────────────────────────────────────────────────────────────

(
"'phases-desc':'Los hitos de fase definen etapas de crecimiento. El multiplicador del límite de riqueza está actualmente fijo en 25× (constante de código Go: wealthCapMultiplier = 25.0) — el ajuste automático por fases es una mejora futura planificada.',",
"'phases-desc':'Los hitos de fase definen etapas de crecimiento. Durante el bootstrap (&lt;25 humanos registrados) el límite usa un multiplicador deslizante: max(5,min(N,25))× promedio — 5× con 1–4 humanos, crece 1× por humano, alcanza 25× completo a partir de 25 humanos. Previene la concentración inicial antes de que exista participación real.',"
),

(
"'p0':'Bootstrap · &lt;100 humanos · Límite de riqueza: 25× saldo promedio · Actualmente activo',",
"'p0':'Bootstrap · &lt;100 humanos · Límite: max(5,min(N,25))× promedio · Deslizante 5×→25× hasta 25 humanos · Activo',"
),

(
"'wealth-cap-explain':'El Límite de Riqueza está actualmente fijado en 25× el saldo promedio de todos los humanos verificados. Es una constante fija en el código Go en vivo. Al ser relativo al promedio actual, se escala automáticamente con el crecimiento de la red.',",
"'wealth-cap-explain':'El Límite de Riqueza escala durante el bootstrap: max(5, min(N, 25)) × saldo promedio, donde N = humanos registrados. Con 1–4 humanos el límite es 5× (5.000 AEQ). Crece 1× por cada nuevo humano. Con 25+ humanos: 25× permanente (25.000 AEQ). Previene la concentración inicial. Siempre relativo al promedio actual.',"
),

(
"'cap-title':'4. LÍMITE DE RIQUEZA — Aplicación de Justicia Matemática','cap-box':'Límite = 25× saldo promedio actual de todos los humanos verificados<br>Se ajusta automáticamente mientras la red crece y los saldos cambian<br>Se aplica a TODAS las direcciones excepto las 4 direcciones del pool de protocolo<br>El exceso de AEQ se redistribuye instantáneamente a los 4 pools de redistribución<br>Sin intervención manual — aplicado a nivel de protocolo en cada transferencia entrante',",
"'cap-title':'4. LÍMITE DE RIQUEZA — Aplicación de Justicia Matemática','cap-box':'Bootstrap: max(5,min(N,25))× saldo promedio AEQ<br>1–4 humanos: 5× (5.000 AEQ) · Crece 1× por humano · 25+: 25× (25.000 AEQ) permanente<br>Se aplica a TODAS las direcciones excepto los 4 pools de protocolo<br>Exceso de AEQ redistribuido instantáneamente · Sin intervención manual',"
),

(
"'ubi-src-cap':'Desbordamiento del Límite','ubi-src-cap-d':'Wallets que superan 25× el saldo promedio son confiscadas al instante. El 20% fluye al UBI.',",
"'ubi-src-cap':'Desbordamiento del Límite','ubi-src-cap-d':'Wallets que superan max(5,min(N,25))× el saldo promedio son confiscadas al instante. El 20% fluye al UBI.',"
),

# ─── Russian ─────────────────────────────────────────────────────────────────

(
"'phases-desc':'Границы фаз определяют вехи роста сети. Мультипликатор лимита богатства в настоящее время зафиксирован на 25× (константа кода Go: wealthCapMultiplier = 25.0) — автоматическая корректировка по фазам запланирована как будущее обновление протокола.',",
"'phases-desc':'Границы фаз определяют вехи роста сети. В фазе запуска (&lt;25 зарегистрированных людей) лимит использует скользящий множитель: max(5,min(N,25))× средний баланс — 5× при 1–4 людях, растёт на 1× за каждого человека, достигает 25× при 25+ людях. Предотвращает концентрацию до реального участия.',"
),

(
"'p0':'Bootstrap · &lt;100 людей · Лимит богатства: 25× средний баланс · Активен сейчас',",
"'p0':'Bootstrap · &lt;100 людей · Лимит: max(5,min(N,25))× среднего · Скользящий 5×→25× до 25 людей · Активен',"
),

(
"'wealth-cap-explain':'Лимит богатства в настоящее время установлен на 25× среднего баланса AEQ всех верифицированных людей. Это фиксированная константа в живом коде Go. Поскольку значение всегда относительно текущего среднего, лимит автоматически масштабируется по мере роста сети.',",
"'wealth-cap-explain':'Лимит богатства масштабируется в фазе запуска: max(5, min(N, 25)) × средний баланс, где N = зарегистрированные люди. При 1–4 людях: 5× (5.000 AEQ). Растёт на 1× за каждого нового человека. При 25+ людях: постоянный 25× (25.000 AEQ). Предотвращает раннюю концентрацию. Всегда относительно текущего среднего.',"
),

(
"'cap-title':'4. ЛИМИТ БОГАТСТВА — Математическое Обеспечение Справедливости','cap-box':'Лимит = 25× текущий средний баланс всех верифицированных людей<br>Автоматически корректируется · Применяется ко всем адресам кроме 4 протокольных пулов<br>Избыточный AEQ мгновенно перераспределяется в 4 пула · Без ручного вмешательства',",
"'cap-title':'4. ЛИМИТ БОГАТСТВА — Математическое Обеспечение Справедливости','cap-box':'Bootstrap: max(5,min(N,25))× средний баланс AEQ<br>1–4 людей: 5× (5.000 AEQ) · +1× за каждого человека · 25+: 25× (25.000 AEQ) постоянно<br>Избыток мгновенно перераспределяется · Применяется ко всем адресам кроме 4 пулов протокола',"
),

(
"'ubi-src-cap':'Превышение Лимита Богатства','ubi-src-cap-d':'Кошельки превышающие 25× средний баланс конфискуются мгновенно. 20% поступает в UBI немедленно.',",
"'ubi-src-cap':'Превышение Лимита Богатства','ubi-src-cap-d':'Кошельки превышающие max(5,min(N,25))× средний баланс конфискуются мгновенно. 20% поступает в UBI.',"
),

# ─── Chinese ─────────────────────────────────────────────────────────────────

(
"'phases-desc':'阶段边界定义网络增长里程碑。财富上限乘数目前固定为25（Go代码常数：wealthCapMultiplier = 25.0）— 基于阶段的自动调整是计划中的未来协议升级。',",
"'phases-desc':'阶段边界定义网络增长里程碑。启动阶段（&lt;25名注册人类）财富上限使用滑动乘数：max(5,min(N,25))×平均余额— 1–4人时为5×，每增加1人增加1×，25名人类质5×完整。防止早期参与者财富集中。',"
),

(
"'p0':'引导期 · &lt;100人类 · 财富上限：25× 平均余额 · 当前激活',",
"'p0':'引导期 · &lt;100人类 · 上限：max(5,min(N,25))×平均 · 滑动5×→25×直25人 · 当前激活',"
),

(
"'wealth-cap-explain':'财富上限目前设定为所有验证人类平均AEQ余额的25倍。这是实时Go代码中的固定常数。由于始终相对于当前平均值，随着网络增长，上限会自动扩展。',",
"'wealth-cap-explain':'财富上限在启动阶段动态调整：max(5, min(N, 25)) × 平均余额，N为已注册人类数。 1–4人时：5×（5,000 AEQ）。1人多1×。25+人时：永久25×（25,000 AEQ）。防止早期采用者在网络形成真实参与前过度积累财富。始终相对于当前平均余额。',"
),

(
"'cap-title':'4. 财富上限 — 数学公平执行','cap-box':'上限 = 所有经过验证的人类当前平均AEQ余额的25倍<br>随网络增长和余额变化自动调整<br>适用于除4个协议池地址外的所有地址<br>超额的AEQ立即重新分配到吹4个再分配池<br>无需手动干预 — 在每次入账转账时在协议级别执行',",
"'cap-title':'4. 财富上限 — 数学公平执行','cap-box':'启动上限：max(5,min(N,25))× 平均AEQ余额<br>1–4人：5×（5,000 AEQ）· 每增1人+1× · 25+人：25×（25,000 AEQ）永久<br>适用于除4个协议池外的所有地址<br>超额AEQ立即重新分配 · 无需手动干预',"
),

(
"'ubi-src-cap':'财富上限溢出','ubi-src-cap-d':'超过25×平均余额的钉包立即被没收超额部分。20%立即流入UBI。',",
"'ubi-src-cap':'财富上限溢出','ubi-src-cap-d':'超过max(5,min(N,25))×平均余额的钉包立即被没收超额部分。20%立即流入UBI。',"
),

# ─── Italian ─────────────────────────────────────────────────────────────────

(
"'phases-desc':'I confini di fase definiscono le tappe di crescita della rete. Il moltiplicatore del limite di ricchezza è attualmente fisso a 25× (costante del codice Go: wealthCapMultiplier = 25.0) — l\'aggiustamento automatico basato sulle fasi è pianificato come aggiornamento futuro del protocollo.',",
"'phases-desc':'I confini di fase definiscono le tappe di crescita della rete. Durante il bootstrap (&lt;25 umani registrati) il limite usa un moltiplicatore scorrevole: max(5,min(N,25))× media — 5× con 1–4 umani, cresce di 1× per ogni nuovo umano, raggiunge il pieno 25× a 25+ umani. Previene la concentrazione iniziale prima che esista una reale partecipazione.',"
),

(
"'p0':'Bootstrap · &lt;100 umani · Limite di Ricchezza: 25× saldo medio · Attualmente attivo',",
"'p0':'Bootstrap · &lt;100 umani · Limite: max(5,min(N,25))× media · Scorrevole 5×→25× fino a 25 umani · Attivo',"
),

(
"'wealth-cap-explain':'Il Limite di Ricchezza è attualmente impostato a 25× il saldo AEQ medio di tutti gli umani verificati. Questa è una costante fissa nel codice Go in produzione. Poiché è sempre relativo alla media corrente, il limite si adatta automaticamente alla crescita della rete.',",
"'wealth-cap-explain':'Il Limite di Ricchezza si adatta durante il bootstrap: max(5, min(N, 25)) × saldo medio, dove N = umani registrati. Con 1–4 umani: 5× (5.000 AEQ). Cresce di 1× per ogni nuovo umano. Con 25+ umani: 25× permanente (25.000 AEQ). Previene la concentrazione iniziale prima che esista reale partecipazione. Sempre relativo alla media corrente.',"
),

(
"'cap-title':'4. LIMITE DI RICCHEZZA — Applicazione dell\'Equità Matematica','cap-box':'Limite = 25× saldo AEQ medio attuale di tutti gli umani verificati<br>Si adatta automaticamente man mano che la rete cresce e i saldi cambiano<br>Si applica a TUTTI gli indirizzi tranne i 4 indirizzi del pool del protocollo<br>L\'eccesso di AEQ viene immediatamente ridistribuito ai 4 pool di redistribuzione<br>Nessun intervento manuale richiesto — applicato a livello di protocollo ad ogni trasferimento in entrata',",
"'cap-title':'4. LIMITE DI RICCHEZZA — Applicazione dell\'Equità Matematica','cap-box':'Bootstrap: max(5,min(N,25))× saldo AEQ medio<br>1–4 umani: 5× (5.000 AEQ) · Cresce 1× per umano · 25+: 25× (25.000 AEQ) permanente<br>Si applica a TUTTI gli indirizzi tranne i 4 pool del protocollo<br>L\'eccesso di AEQ viene immediatamente ridistribuito · Nessun intervento manuale',"
),

(
"'ubi-src-cap':'Overflow Limite di Ricchezza','ubi-src-cap-d':'I wallet che superano 25× il saldo medio hanno l\'eccesso confiscato istantaneamente. Il 20% fluisce all\'UBI immediatamente.',",
"'ubi-src-cap':'Overflow Limite di Ricchezza','ubi-src-cap-d':'I wallet che superano max(5,min(N,25))× il saldo medio hanno l\'eccesso confiscato istantaneamente. Il 20% fluisce all\'UBI.',"
),

# ─── Indonesian ──────────────────────────────────────────────────────────────

(
"'phases-desc':'Batas fase mendefinisikan tonggak pertumbuhan jaringan. Pengganda batas kekayaan saat ini ditetapkan pada 25× (konstanta kode Go: wealthCapMultiplier = 25.0) — penyesuaian otomatis berbasis fase direncanakan untuk peningkatan protokol mendatang.',",
"'phases-desc':'Batas fase mendefinisikan tonggak pertumbuhan jaringan. Selama bootstrap (&lt;25 manusia terdaftar) batas menggunakan pengganda geser: max(5,min(N,25))× rata-rata — 5× dengan 1–4 manusia, bertambah 1× per manusia baru, mencapai 25× penuh pada 25+ manusia. Mencegah konsentrasi awal sebelum partisipasi nyata terbentuk.',"
),

(
"'p0':'Bootstrap · &lt;100 manusia · Batas Kekayaan: 25× saldo rata-rata · Saat ini aktif',",
"'p0':'Bootstrap · &lt;100 manusia · Batas: max(5,min(N,25))× rata-rata · Geser 5×→25× hingga 25 manusia · Aktif',"
),

(
"'wealth-cap-explain':'Batas Kekayaan saat ini ditetapkan pada 25× saldo AEQ rata-rata semua manusia terverifikasi. Ini adalah konstanta tetap dalam kode Go langsung. Karena selalu relatif terhadap rata-rata saat ini, batas secara otomatis diskalakan seiring pertumbuhan jaringan.',",
"'wealth-cap-explain':'Batas Kekayaan diskalakan selama bootstrap: max(5, min(N, 25)) × saldo rata-rata, di mana N = manusia terdaftar. Dengan 1–4 manusia: 5× (5.000 AEQ). Bertambah 1× per manusia baru. Dengan 25+ manusia: 25× permanen (25.000 AEQ). Mencegah konsentrasi awal sebelum partisipasi nyata ada. Selalu relatif terhadap rata-rata saat ini.',"
),

(
"'cap-title':'4. BATAS KEKAYAAN — Penerapan Keadilan Matematis','cap-box':'Batas = 25× saldo AEQ rata-rata semua manusia terverifikasi saat ini<br>Otomatis menyesuaikan seiring pertumbuhan jaringan dan perubahan saldo<br>Berlaku untuk SEMUA alamat kecuali 4 alamat pool protokol<br>Kelebihan AEQ langsung didistribusikan ulang ke 4 pool redistribusi<br>Tanpa intervensi manual — diterapkan di tingkat protokol pada setiap transfer masuk',",
"'cap-title':'4. BATAS KEKAYAAN — Penerapan Keadilan Matematis','cap-box':'Bootstrap: max(5,min(N,25))× saldo AEQ rata-rata<br>1–4 manusia: 5× (5.000 AEQ) · +1× per manusia · 25+: 25× (25.000 AEQ) permanen<br>Berlaku untuk SEMUA alamat kecuali 4 pool protokol<br>Kelebihan AEQ langsung didistribusikan ulang · Tanpa intervensi manual',"
),

(
"'ubi-src-cap':'Overflow Batas Kekayaan','ubi-src-cap-d':'Dompet melebihi 25× saldo rata-rata langsung disita kelebihannya. 20% mengalir ke UBI segera.',",
"'ubi-src-cap':'Overflow Batas Kekayaan','ubi-src-cap-d':'Dompet melebihi max(5,min(N,25))× saldo rata-rata langsung disita kelebihannya. 20% mengalir ke UBI.',"
),

]

errors = []
for old, new in REPLACEMENTS:
    if old not in content:
        errors.append(f'NOT FOUND: {old[:80]}...')
    else:
        content = content.replace(old, new, 1)

if errors:
    with open('patch_wealth_cap_errors.txt', 'w', encoding='utf-8') as ef:
        ef.write('\n'.join(errors))
    print(f'ERRORS: {len(errors)} strings not found. See patch_wealth_cap_errors.txt')
    sys.exit(1)

if content == original:
    print('ERROR: no changes made!')
    sys.exit(1)

with open(TARGET, 'wb') as f:
    f.write(content.encode('utf-8'))

print(f'Done! {len(REPLACEMENTS)} replacements applied.')
