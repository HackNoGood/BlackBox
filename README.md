# 🖤 BlackBox
**A decentralized, retro-style peer-to-peer terminal chat.**

BlackBox is a modern reimagining of 1980s bulletin board systems (BBS) — rebuilt for the decentralized internet age.  
It combines encrypted peer-to-peer communication, nostalgic CRT aesthetics, and modern libp2p networking to create a truly private hangout in the terminal.

---

## ⚡ Features
- 🔒 **End-to-End Encrypted Chat** using [libp2p](https://libp2p.io/)
- 🌍 **Peer-to-Peer Architecture** — no servers, no middlemen
- 🖥️ **Retro CRT Terminal Interface** with ASCII banner boot sequence
- 🚪 **Host or Join Modes** — run your own “Black Site” or connect to a friend’s
- 🧩 **AutoRelay Support** for NAT traversal (no port-forwarding needed)
- 🧠 **Lightweight Go Binary** — no dependencies beyond Go itself

---

## 🧠 Concept
BlackBox acts like a digital underground chatroom — private, ephemeral, and fully off-grid.  
Each host node becomes its own self-contained “Black Site.”  
Users can share their connection address (or QR code) to allow others to join directly.

When the host disconnects, the Black Site disappears.

---

## 🧰 Installation
### Clone & Build
```bash
git clone https://github.com/HackNoGood/BlackBox.git
cd BlackBox
go mod tidy
go build -o blackbox
