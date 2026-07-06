// WARNING (P3-k, audit 2026-07-06): this is a stale leftover, most likely
// from before the V7 rewrite/rename -- it deploys a contract literally
// named "Aequitas", which does not exist anywhere in contracts/ (only
// AequitasV7.sol does). Running this via `hardhat run scripts/deploy.js`
// will fail immediately, not silently deploy something wrong -- but its
// innocuous name is exactly the kind of thing a stressed operator might
// reach for during a launch-night deploy instead of the REAL deploy
// script, deploy_v7.cjs (repo root, run directly with `node`, not via
// `hardhat run`). Left in place rather than deleted so git history/blame
// stays intact, but treat this file as dead: use deploy_v7.cjs instead.
import { network } from "hardhat";
import { parseEther } from "viem";

const { viem } = await import("hardhat");

const aequitas = await viem.deployContract("Aequitas");
console.log("Aequitas deployed to:", aequitas.address);

const [humans, supply, cap] = await aequitas.read.getStatus();
console.log("Humans: ", humans.toString());
console.log("Total Supply:", supply.toString());
console.log("Max Cap: ", cap.toString());
console.log("\nAequitas laeuft auf der Chain.");