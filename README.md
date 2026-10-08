# DogeVM Connect

Local Go wallet + web UI for [DogecoinVM](https://metaldoge.com/) on Metal Blockchain ([Metallicus](https://metallicus.com/)).

One Dogecoin key works on **Dogecoin mainnet** and **DogecoinVM**. Move DOGE across the official bridge, track both chains in one place, and compare live speed on dual gauges (VM finality + Dogecoin header cadence).

## Features

- Create or import a Dogecoin wallet (WIF / hex), encrypted on disk with a passphrase
- Same `D…` address on L1 and DogecoinVM - balances and UTXOs kept separate and labeled
- Send on either chain (local signing; keys never leave your machine)
- Bridge **deposit** (Dogecoin → DogecoinVM) with redeem-script verification
- Bridge **withdraw** (DogecoinVM → Dogecoin) via `DVMO` OP_RETURN tag
- Dual-chain transaction history + public bridge activity feed
- Dual speedometers: DogecoinVM payment finality + Dogecoin mainnet header intervals from tip events
- Tip events via `/api/events`, plus confirmation and spendable-balance polling on both chains
- Sidebar for Home, Wallet, Activity, Settings, API docs, and Learn
- Optional user and admin API tokens
- HTTP API for wallet create, unlock, L1 send, DogecoinVM send, bridge both ways, and confirmation polling
- UI: day/night split, Material Symbols, wallet wizard (send/receive/bridge), QR codes, first-use terms gate

Built against the public [DogecoinVM API](https://metaldoge.com/developers).

## Quick start

```bash
go run ./cmd/dogevm-connect
```

Or compile with the website embedded (Windows):

```powershell
powershell -ExecutionPolicy Bypass -File scripts\build.ps1
.\dist\dogevm-connect.exe
```

Extra targets: `.\scripts\build.ps1 -All`

GitHub Releases also publish those binaries. See [Releases](#releases).

Opens `http://127.0.0.1:8787/` as **DogeVM Connect**. Flags:

| Flag | Default | Meaning |
| --- | --- | --- |
| `-addr` | `127.0.0.1:8787` | HTTP listen address |
| `-api` | `https://metaldoge.com` | API base URL |
| `-data` | OS user config dir | Encrypted wallet directory |
| `-no-browser` | off | Do not open a browser |
| `-user-token` | empty | User API token for this process |
| `-admin-token` | empty | Admin API token for this process |

Wallet file: `wallet.json` under the data directory (`%AppData%\DogeVMConnect\data` on Windows, `~/.dogevm-connect/data` elsewhere).

Token file: `connect_config.json` in that same directory. You can also set tokens in the sidebar under **Settings**. Environment variables `DOGEVM_USER_TOKEN` and `DOGEVM_ADMIN_TOKEN` override the file when the process starts. Flags override both for that process only.

## Releases

A push to `main` creates the next patch tag, starting at `v1.0.0`, and [GitHub Actions](.github/workflows/release.yml) compiles DogeVM Connect for Windows, macOS, and Linux (amd64 and arm64). The binaries are attached to that GitHub Release, with `SHA256SUMS`.

Pushing a `v*` tag yourself, or running the **Release** workflow by hand with a version such as `1.2.3`, publishes that version instead.

Each file is named `dogevm-connect-v{version}-{os}-{arch}`, and Windows builds end in `.exe`. Check a download with:

```bash
sha256sum -c SHA256SUMS
```

## Web interface

The left sidebar is a persistent menu, with the page on the right.

- **Home**: speed gauges and chain tips
- **Wallet**: create, import, unlock, send, receive, bridge
- **Activity**: your transactions and the public bridge feed
- **Settings**: user token and admin token
- **API docs**: curl and JavaScript examples for every wallet call
- **Learn**: what is different about Dogecoin L1 and DogecoinVM

The first visit shows a terms gate. The background stays blurred until you turn on the switch and choose Enter. The switch stays off until you accept. Terms: [https://metaldoge.com/terms](https://metaldoge.com/terms).

## Access tokens

Leave both tokens empty (the default) and every call on this computer is allowed. That matches a local desktop app.

When you set tokens:

| Token | Can do |
| --- | --- |
| User | Create or import a wallet, unlock, lock, read balances, send on Dogecoin L1, send on DogecoinVM, bridge both ways, read confirmations |
| Admin | Everything the user token can do, plus export the key, delete the wallet, and change tokens |

The two tokens must be different, and each one at least 8 characters with no spaces. A user token cannot be saved unless an admin token is set too.

Send the token in any of these ways:

- `Authorization: Bearer TOKEN`
- `X-Token: TOKEN`
- `?token=TOKEN` (useful for the live tip stream)
- path prefix `http://127.0.0.1:8787/TOKEN/api/...`

`GET /api/health` and `GET /api/auth` stay open so you can see whether a token is required (`{"authRequired":true,"role":"none"}`).

## HTTP API

Base URL below is `http://127.0.0.1:8787`. Replace `YOUR_TOKEN` with the user or admin token. Omit the `Authorization` header when tokens are off.

The wallet must be **unlocked** before send or bridge. `GET /api/balances` also needs an unlock because it uses the in-memory session. `GET /api/confirm` works from the address stored on disk.

Add `"preview": true` on send, deposit, or withdraw to get the fee breakdown without broadcasting.

The same `D...` address exists on both chains. A Dogecoin L1 send does not move coins onto DogecoinVM. Only `POST /api/bridge/deposit` does that.

### Generate a wallet

```bash
curl -s -X POST http://127.0.0.1:8787/api/wallet/create \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"passphrase\":\"choose a long passphrase\"}"
```

```javascript
const created = await fetch("http://127.0.0.1:8787/api/wallet/create", {
  method: "POST",
  headers: {
    "Content-Type": "application/json",
    Authorization: "Bearer YOUR_TOKEN",
  },
  body: JSON.stringify({ passphrase: "choose a long passphrase" }),
}).then((r) => r.json());
console.log(created.address);
```

### Unlock

```bash
curl -s -X POST http://127.0.0.1:8787/api/wallet/unlock \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"passphrase\":\"choose a long passphrase\"}"
```

```javascript
await fetch("http://127.0.0.1:8787/api/wallet/unlock", {
  method: "POST",
  headers: {
    "Content-Type": "application/json",
    Authorization: "Bearer YOUR_TOKEN",
  },
  body: JSON.stringify({ passphrase: "choose a long passphrase" }),
});
```

### Balances

`spendable` is the sum of UTXOs with at least 1 confirmation, in DOGE.

```bash
curl -s http://127.0.0.1:8787/api/balances \
  -H "Authorization: Bearer YOUR_TOKEN"
```

```javascript
const bal = await fetch("http://127.0.0.1:8787/api/balances", {
  headers: { Authorization: "Bearer YOUR_TOKEN" },
}).then((r) => r.json());
console.log(bal.chains.dogecoin.spendable, bal.chains.dogecoinvm.spendable);
```

### Send Dogecoin L1 to another L1 address

```bash
curl -s -X POST http://127.0.0.1:8787/api/send \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"chain\":\"dogecoin\",\"to\":\"Dxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx\",\"amount\":\"1.5\"}"
```

```javascript
const sentL1 = await fetch("http://127.0.0.1:8787/api/send", {
  method: "POST",
  headers: {
    "Content-Type": "application/json",
    Authorization: "Bearer YOUR_TOKEN",
  },
  body: JSON.stringify({
    chain: "dogecoin",
    to: "Dxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
    amount: "1.5",
  }),
}).then((r) => r.json());
```

### Send DogecoinVM to another DogecoinVM address

```bash
curl -s -X POST http://127.0.0.1:8787/api/send \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"chain\":\"dogecoinvm\",\"to\":\"Dyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyy\",\"amount\":\"1.5\"}"
```

```javascript
const sentVM = await fetch("http://127.0.0.1:8787/api/send", {
  method: "POST",
  headers: {
    "Content-Type": "application/json",
    Authorization: "Bearer YOUR_TOKEN",
  },
  body: JSON.stringify({
    chain: "dogecoinvm",
    to: "Dyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyy",
    amount: "1.5",
  }),
}).then((r) => r.json());
```

### Bridge Dogecoin L1 to DogecoinVM

Pays your verified deposit address on Dogecoin. After enough L1 confirmations the bridge credits this same address on DogecoinVM, minus `vmFee`.

```bash
curl -s -X POST http://127.0.0.1:8787/api/bridge/deposit \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"amount\":\"10\"}"
```

```javascript
const deposit = await fetch("http://127.0.0.1:8787/api/bridge/deposit", {
  method: "POST",
  headers: {
    "Content-Type": "application/json",
    Authorization: "Bearer YOUR_TOKEN",
  },
  body: JSON.stringify({ amount: "10" }),
}).then((r) => r.json());
```

### Bridge DogecoinVM back to Dogecoin L1

`to` is the Dogecoin address that receives the payout.

```bash
curl -s -X POST http://127.0.0.1:8787/api/bridge/withdraw \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"to\":\"Dzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz\",\"amount\":\"5\"}"
```

```javascript
const withdraw = await fetch("http://127.0.0.1:8787/api/bridge/withdraw", {
  method: "POST",
  headers: {
    "Content-Type": "application/json",
    Authorization: "Bearer YOUR_TOKEN",
  },
  body: JSON.stringify({
    to: "Dzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz",
    amount: "5",
  }),
}).then((r) => r.json());
```

### Confirmations and available balance

Poll `GET /api/confirm` until `confirmed` and `available` are both true.

| `flow` | What you pass | `confirmed` | `available` |
| --- | --- | --- | --- |
| `send` | `chain=dogecoin` or `chain=dogecoinvm`, plus `txid` | At least 1 confirmation on that chain | Confirmed, and this wallet still has a spendable balance on that chain |
| `deposit` | Dogecoin deposit `txid` | Deposit reached the required Dogecoin confirmations, or the bridge already credited | Bridge credit exists and DogecoinVM spendable balance is above 0 |
| `withdraw` | DogecoinVM withdraw `txid` | Dogecoin payout has at least 1 confirmation | Same as confirmed. `balance` is this wallet on L1, so it moves only if you withdrew to yourself |

```bash
curl -s "http://127.0.0.1:8787/api/confirm?flow=send&chain=dogecoin&txid=TXID" \
  -H "Authorization: Bearer YOUR_TOKEN"

curl -s "http://127.0.0.1:8787/api/confirm?flow=deposit&txid=L1_TXID" \
  -H "Authorization: Bearer YOUR_TOKEN"

curl -s "http://127.0.0.1:8787/api/confirm?flow=withdraw&txid=VM_TXID" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

```javascript
async function waitReady(params) {
  const headers = { Authorization: "Bearer YOUR_TOKEN" };
  for (let i = 0; i < 120; i++) {
    const q = new URLSearchParams(params);
    const data = await fetch("http://127.0.0.1:8787/api/confirm?" + q, { headers })
      .then((r) => r.json());
    if (data.confirmed && data.available) return data;
    await new Promise((r) => setTimeout(r, 3000));
  }
  throw new Error("timed out waiting for confirmation");
}

await waitReady({ flow: "send", chain: "dogecoin", txid: sentL1.txid });
await waitReady({ flow: "send", chain: "dogecoinvm", txid: sentVM.txid });
await waitReady({ flow: "deposit", txid: deposit.txid });
await waitReady({ flow: "withdraw", txid: withdraw.txid });
```

### Other routes

| Method | Path | Who | Purpose |
| --- | --- | --- | --- |
| GET | `/api/health` | public | Process is up |
| GET | `/api/auth` | public | `authRequired` and `role` |
| GET, POST | `/api/config` | admin | Read or save `user_token` and `admin_token` |
| GET | `/api/wallet` | user | Address, exists, unlocked |
| POST | `/api/wallet/create` | user | Generate a wallet |
| POST | `/api/wallet/import` | user | Import WIF or hex (`wif` or `hex`, plus `passphrase`) |
| POST | `/api/wallet/unlock` | user | Unlock |
| POST | `/api/wallet/lock` | user | Lock |
| POST | `/api/wallet/export` | admin | WIF and hex of the unlocked key |
| POST | `/api/wallet/remove` | admin | Delete the wallet file |
| GET | `/api/balances` | user | Both chains, including `spendable` |
| GET | `/api/transactions` | user | History on both chains with `confirmations` |
| POST | `/api/send` | user | `chain`, `to`, `amount`, optional `preview` |
| GET | `/api/confirm` | user | `flow`, `txid`, and `chain` when `flow=send` |
| POST | `/api/bridge/deposit` | user | L1 to DogecoinVM. Body: `amount` |
| GET | `/api/bridge/deposit-address` | user | Verified deposit address |
| GET | `/api/bridge/deposits` | user | Deposit rows for this address |
| POST | `/api/bridge/withdraw` | user | DogecoinVM to L1. Body: `to`, `amount` |
| GET | `/api/bridge/pegout?txid=` | user | Payout status for a withdraw tx |
| POST | `/api/doge/import` | user | Ask the index to learn an older L1 txid |

## Safety

- Alpha: DogecoinVM and its bridge are experimental and capped. Read the [terms](https://metaldoge.com/terms).
- Never send DOGE on Dogecoin to your normal `D…` address expecting a bridge credit - that stays on L1. Only the **deposit address** bridges.
- Always confirm which chain you are signing for before broadcast.
- Back up exported WIF offline. Lost keys cannot be recovered.

## Architecture

```
cmd/dogevm-connect     entrypoint
internal/wallet     keygen, WIF, AES-GCM encrypted store
internal/dogetx     UTXO select, legacy P2PKH sign, fee rules
internal/bridge     deposit redeem-script check, DVMO tags
internal/metalapi   metaldoge.com JSON client
internal/tracker    finality samples + tip SSE
internal/config     user and admin API tokens
internal/server     local HTTP API
web/static          Dogecoin-gold UI + speedometer
```

Payments are tracked in this app: balances, history, and `/api/confirm` on Dogecoin L1 and DogecoinVM, including bridge deposits and withdrawals. No separate watcher is required.

## License

MIT. Copyright (c) 2026 Paulo Vidal (https://x.com/inevitable360, https://github.com/qlpqlp). See [LICENSE](LICENSE).
