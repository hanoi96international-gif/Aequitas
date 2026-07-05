import hardhatToolboxViemPlugin from "@nomicfoundation/hardhat-toolbox-viem";
import { configVariable, defineConfig } from "hardhat/config";

export default defineConfig({
  plugins: [hardhatToolboxViemPlugin],
  solidity: {
    profiles: {
      // FIX (P1-1, beta-launch audit 2026-07-05): `default` used to have no
      // optimizer settings at all, unlike `production` — `npx hardhat test`
      // (the obvious, documented default command, no flags) compiled
      // AequitasV7.sol to 24,630 bytes of runtime bytecode, 54 bytes over
      // EIP-170's 24,576 limit. Every deploy in the Guardian/UBI regression
      // suite failed at the constructor with a generic "contract code too
      // large" RPC error — the project's most safety-critical test file ran
      // with zero real coverage unless someone remembered
      // --build-profile production. Production bytecode itself was never at
      // risk (11,379 bytes, well under the limit) — this was purely a
      // silently-broken-by-default test harness. Both profiles now compile
      // identically so there is no more "wrong way" to run the tests.
      default: {
        version: "0.8.28",
        settings: {
          optimizer: {
            enabled: true,
            runs: 200,
          },
        },
      },
      production: {
        version: "0.8.28",
        settings: {
          optimizer: {
            enabled: true,
            runs: 200,
          },
        },
      },
    },
  },
  networks: {
    hardhatMainnet: {
      type: "edr-simulated",
      chainType: "l1",
    },
    hardhatOp: {
      type: "edr-simulated",
      chainType: "op",
    },
    sepolia: {
      type: "http",
      chainType: "l1",
      url: configVariable("SEPOLIA_RPC_URL"),
      accounts: [configVariable("SEPOLIA_PRIVATE_KEY")],
    },
  },
});
