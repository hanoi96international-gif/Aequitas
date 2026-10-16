import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { readFileSync } from "node:fs";
import path from "node:path";
import vm from "node:vm";
import { fileURLToPath } from "node:url";
import { Wallet, getBytes, toBeHex } from "ethers";

// Die Website (explorer.js, vom Knoten ausgeliefert) muss fuer V8 genau das
// unterschreiben, was jeder Knoten prueft. Der Block zwischen
// V8-REGISTER-BEGIN/END wird hier mit dem AUSGELIEFERTEN ethers
// (assets/vendor/ethers.min.js) ausgefuehrt und gegen den gemeinsamen
// Pruefvektor von Go (TestV8_GoldenVektorMitDerApp) und App geprueft.

const here = path.dirname(fileURLToPath(import.meta.url));
const assets = path.join(here, "..", "x", "humanity", "keeper", "assets");

function ladeBlock() {
  const js = readFileSync(path.join(assets, "explorer.js"), "utf8");
  const start = js.indexOf("// V8-REGISTER-BEGIN");
  const ende = js.indexOf("// V8-REGISTER-END");
  assert.ok(start > 0 && ende > start, "V8-Block in explorer.js fehlt");
  const v7 = /const V7_CONTRACT = '(0x[0-9a-fA-F]{40})'/.exec(js);
  assert.ok(v7, "V7_CONTRACT fehlt");
  const ctx: any = { module: undefined, exports: undefined, define: undefined, TextEncoder, TextDecoder, crypto: globalThis.crypto, console };
  vm.createContext(ctx);
  // Wie im Browser: ethers sucht sein globales Objekt ueber self/window.
  vm.runInContext("globalThis.self = globalThis; globalThis.window = globalThis;", ctx);
  vm.runInContext(readFileSync(path.join(assets, "vendor", "ethers.min.js"), "utf8"), ctx);
  assert.ok(ctx.ethers?.TypedDataEncoder, "ausgeliefertes ethers ohne TypedDataEncoder");
  const kStart = js.indexOf("// CHAIN-ID-BEGIN");
  const kEnde = js.indexOf("// CHAIN-ID-END");
  assert.ok(kStart >= 0 && kEnde > kStart, "CHAIN-ID-Block in explorer.js fehlt");
  vm.runInContext(
    `const V7_CONTRACT = '${v7[1]}';\n${js.slice(kStart, kEnde)}\n${js.slice(start, ende)}\n` +
      `globalThis.V8 = { v8RegisterVertrag, v8TypedData, v8Signatur, v8Unterschreiben, V8_FRIST_S, aeqChainIdPruefen, aeqChainHex, AEQ_CHAIN_IDS };`,
    ctx,
  );
  return { V8: ctx.V8, ethers: ctx.ethers };
}

const { V8, ethers } = ladeBlock();
const KENNUNG = "aequitas-1926-1790000000";
const ALICE = new Wallet("0x" + "11".repeat(32));
const MALLORY = new Wallet("0x" + "22".repeat(32));
const SIGNALE = ["1001", "2001"];
const FRIST = 1_790_000_600;
const DIGEST = "0x0cfd835d1262f048ab9e759f064e7d6ebfb747599e40a606e5cd84dc13cc1804";
const SIG =
  "0x9d5387fee26b5ec00237ff732f68821b31744e07c81b8b815f048c1c9ad8965d4032040a16fbf1ef18bd9c24bbdab9c76df4c03fa8637ce003f18a4dc4d05cd31c";
const N = 0xfffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141n;
// Chain-ID-Wechsel (docs/CHAIN_ID_WECHSEL.md): derselbe Vektor mit 19260930,
// gleich zu Go (TestV8_GoldenVektorNeueChainID) und App.
const CID = 1926;
const CID_NEU = 19260930;
const DIGEST_NEU = "0x7e1718f2076ab61ab691b4c469089eadca8e2e78f2219e2aa8e35989e0038953";
const SIG_NEU =
  "0x4616f2684cba8c7e02a48639f296f981b2a16c43ae47b4917041ca8beedb0f3347531350344bf14880c2a854c8602d8b2666b4e69cd219f936dff02cdde9f7651c";

// Eine Wallet wie MetaMask: nimmt das JSON von eth_signTypedData_v4 und
// unterschreibt es mit ihrem Schluessel.
function wallet(w: Wallet, verbiegen: (s: string) => string = (s) => s) {
  const calls: any[] = [];
  return {
    calls,
    request: async ({ method, params }: any) => {
      calls.push({ method, params });
      assert.equal(method, "eth_signTypedData_v4");
      const p = JSON.parse(params[1]);
      const { EIP712Domain: _, ...types } = p.types;
      return verbiegen(await w.signTypedData(p.domain, types, p.message));
    },
  };
}

describe("explorer.js: V8-Registrierung (EIP-712)", () => {
  it("Pruefvektor: Digest und Signatur byte-gleich zu Go und App", async () => {
    const td = V8.v8TypedData(KENNUNG, ALICE.address, SIGNALE, FRIST, CID);
    assert.equal(ethers.TypedDataEncoder.hash(td.domain, td.types, td.message), DIGEST);
    const w = wallet(ALICE);
    assert.equal(await V8.v8Unterschreiben(w, KENNUNG, ALICE.address.toLowerCase(), SIGNALE, FRIST, CID), SIG);
    const p = JSON.parse(w.calls[0].params[1]);
    assert.deepEqual(p.types.EIP712Domain.map((f: any) => f.name), ["name", "version", "chainId", "verifyingContract", "salt"]);
    assert.equal(p.message.nonce, "0");
  });

  it("gleicht v 0/1 an", async () => {
    const w = wallet(ALICE, (s) => s.slice(0, -2) + (parseInt(s.slice(-2), 16) - 27).toString(16).padStart(2, "0"));
    assert.equal(await V8.v8Unterschreiben(w, KENNUNG, ALICE.address, SIGNALE, FRIST, CID), SIG);
  });

  it("fremde Wallet hinter der Adresse: abgelehnt", async () => {
    await assert.rejects(V8.v8Unterschreiben(wallet(MALLORY), KENNUNG, ALICE.address, SIGNALE, FRIST, CID), /not from this wallet/);
  });

  it("Unterschrift aus anderem Netz oder fuer andere Signale: abgelehnt", async () => {
    const alt = await ALICE.signTypedData(
      ...(() => { const t = V8.v8TypedData("aequitas-1926-1700000000", ALICE.address, SIGNALE, FRIST, CID); return [t.domain, t.types, t.message] as const; })(),
    );
    const td = V8.v8TypedData(KENNUNG, ALICE.address, SIGNALE, FRIST, CID);
    assert.throws(() => V8.v8Signatur(td, alt), /not from this wallet/);
    const vertauscht = V8.v8TypedData(KENNUNG, ALICE.address, ["2001", "1001"], FRIST, CID);
    assert.throws(() => V8.v8Signatur(vertauscht, SIG), /not from this wallet/);
  });

  it("hohes s, falsche Laenge, falsches v: abgelehnt", () => {
    const td = V8.v8TypedData(KENNUNG, ALICE.address, SIGNALE, FRIST, CID);
    const b = getBytes(SIG);
    const s = BigInt("0x" + SIG.slice(66, 130));
    const formbar = SIG.slice(0, 66) + toBeHex(N - s, 32).slice(2) + (b[64] === 27 ? "1c" : "1b");
    assert.throws(() => V8.v8Signatur(td, formbar), /high s/);
    assert.throws(() => V8.v8Signatur(td, SIG.slice(0, -2)), /65 bytes/);
    assert.throws(() => V8.v8Signatur(td, SIG.slice(0, -2) + "1d"), /invalid v/);
  });

  it("ungueltige Eingaben: nicht unterschreiben", () => {
    const r = "21888242871839275222246405745257275088548364400416034343698204186575808495617";
    for (const [k, sig] of [
      ["aequitas-1-1790000000", SIGNALE],
      [undefined, SIGNALE],
      [KENNUNG, ["0", "1"]],
      [KENNUNG, ["1", r]],
      [KENNUNG, ["0x10", "2"]],
      [KENNUNG, ["1"]],
    ] as const) {
      assert.throws(() => V8.v8TypedData(k, ALICE.address, sig, FRIST, CID));
    }
    assert.throws(() => V8.v8TypedData(KENNUNG, "0x1234", SIGNALE, FRIST, CID));
    assert.throws(() => V8.v8TypedData(KENNUNG, ALICE.address, SIGNALE, 0, CID));
  });

  it("Vertragsversion: fehlend = v7, unbekannt = null; Frist unter einem Tag", () => {
    assert.equal(V8.v8RegisterVertrag({}), "v7");
    assert.equal(V8.v8RegisterVertrag({ register_vertrag: "v8" }), "v8");
    assert.equal(V8.v8RegisterVertrag({ register_vertrag: "v9" }), null);
    assert.equal(V8.v8RegisterVertrag(null), null);
    assert.ok(V8.V8_FRIST_S > 15 * 60 && V8.V8_FRIST_S < 24 * 60 * 60);
  });

  it("Chain-ID-Wechsel: Pruefvektor fuer 19260930, und keine Wiederholung ueber den Wechsel", async () => {
    const td = V8.v8TypedData(KENNUNG, ALICE.address, SIGNALE, FRIST, CID_NEU);
    assert.equal(ethers.TypedDataEncoder.hash(td.domain, td.types, td.message), DIGEST_NEU);
    assert.equal(await V8.v8Unterschreiben(wallet(ALICE), KENNUNG, ALICE.address, SIGNALE, FRIST, CID_NEU), SIG_NEU);
    // Eine 1926-Unterschrift gilt nicht fuer die neue Domaene und umgekehrt.
    assert.throws(() => V8.v8Signatur(td, SIG), /not from this wallet/);
    assert.throws(() => V8.v8Signatur(V8.v8TypedData(KENNUNG, ALICE.address, SIGNALE, FRIST, CID), SIG_NEU), /not from this wallet/);
    // Netzkennung bleibt eingefroren auf 1926.
    assert.throws(() => V8.v8TypedData("aequitas-19260930-1790000000", ALICE.address, SIGNALE, FRIST, CID_NEU), /network id/);
  });

  it("Chain-ID nur aus der festen Liste", () => {
    for (const cid of [1, 0, 1927, 19260931, "1926", 1926n, undefined, null, NaN]) {
      assert.throws(() => V8.v8TypedData(KENNUNG, ALICE.address, SIGNALE, FRIST, cid), /chain id/);
    }
  });

  it("aeqChainIdPruefen: Status und eth_chainId muessen dieselbe ID der Liste melden", () => {
    const AB = 1_800_000_000;
    assert.equal(V8.aeqChainIdPruefen({ chain_evm_id: 1926, chain_id_wechsel_ab: 0 }, "0x786"), 1926);
    assert.equal(V8.aeqChainIdPruefen({ chain_evm_id: 1926 }, "0x786"), 1926);
    assert.equal(V8.aeqChainIdPruefen({ chain_evm_id: 19260930, chain_id_wechsel_ab: AB }, "0x125E602"), 19260930);
    for (const [st, rpc] of [
      [{ chain_evm_id: 1926 }, "0x125e602"], // uneinig
      [{ chain_evm_id: 19260930, chain_id_wechsel_ab: AB }, "0x786"], // uneinig
      [{ chain_evm_id: 19260930 }, "0x125e602"], // neue ID ohne Stichtag
      [{ chain_evm_id: 1 }, "0x1"], // fremde Kette
      [{ chain_evm_id: "1926" }, "0x786"], // Text statt Zahl
      [{ chain_evm_id: 1926, chain_id_wechsel_ab: -1 }, "0x786"],
      [{ chain_evm_id: 1926, chain_id_wechsel_ab: "x" }, "0x786"],
      [{ chain_evm_id: 1926 }, undefined], // RPC ohne Ergebnis
      [null, "0x786"], // Knoten unerreichbar
    ] as const) {
      assert.equal(V8.aeqChainIdPruefen(st, rpc), null, JSON.stringify(st) + " " + rpc);
    }
    assert.equal(V8.aeqChainHex(19260930), "0x125e602");
    assert.throws(() => V8.aeqChainHex(1));
  });
});
