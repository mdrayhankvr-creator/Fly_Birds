# Fly_Birds on a free cloud server · ফ্রি ক্লাউড সার্ভারে Fly_Birds

How to put one school online, with HTTPS, on a cloud machine that costs nothing — for a demo, a pilot, or a small school's first year. Plan about an hour, most of it waiting for things to install.

একটি স্কুলকে HTTPS সহ অনলাইনে আনার উপায়, এমন একটি ক্লাউড মেশিনে যার জন্য কোনো টাকা লাগে না — ডেমো, পাইলট, বা ছোট স্কুলের প্রথম বছরের জন্য। প্রায় এক ঘণ্টা সময় লাগবে, যার বেশিরভাগই ইনস্টল হওয়ার অপেক্ষা।

**The English guide is sections 1–12 below. বাংলা নির্দেশনা ১৩ নম্বর অংশ থেকে শুরু, একই ক্রমে (১ = 1, ২ = 2, …)।**

This guide is the free-hosting path. For a paid server, several schools on one machine, upgrades, rollback and secret rotation, use [HOSTING_GUIDE.md](HOSTING_GUIDE.md), [DEPLOYMENT.md](DEPLOYMENT.md) and [OPERATIONS.md](OPERATIONS.md); nothing here contradicts them.

---

## 1. What you must do yourself

Nobody can create these for you. Have them ready before you start.

| # | What | Where | Cost |
|---|---|---|---|
| 1 | A cloud account with a free tier | [Oracle Cloud](https://www.oracle.com/cloud/free/) (recommended), or Google Cloud | Free; Oracle asks for a card to verify identity and does not charge for Always Free resources |
| 2 | An SSH key pair | Made on your laptop (section 3) | Free |
| 3 | A free subdomain, **or** your own domain | [DuckDNS](https://www.duckdns.org) for free, or your registrar | Free / the domain's price |
| 4 | An email provider account for the "forgot password" link | [Brevo](https://www.brevo.com) free plan, 300 emails a day | Free; optional |
| 5 | Values you choose: the school code, the school's domain, an address for certificate notices | — | — |

Oracle's Always Free ARM machine (**VM.Standard.A1.Flex**, up to 4 cores and 24 GB) is the one to pick: it is well above what one school needs, it does not expire after 12 months, and every image in this stack is built for ARM. If ARM capacity is unavailable in your region, an Always Free **VM.Standard.E2.1.Micro** (1 core, 1 GB) also runs a small school, but give it 2 GB of swap and expect image builds to be slow.

**Never commit `deploy/<code>.env`.** It holds the school's database password and signing key. It is already in `.gitignore`.

---

## 2. Create the machine

1. In the cloud console, create a **Compute instance**.
2. Image: **Ubuntu 24.04 LTS** (the `aarch64`/ARM build for an A1 machine).
3. Shape: **VM.Standard.A1.Flex**, 2 cores and 8 GB is plenty (4 and 24 are free too).
4. Boot volume: **50 GB** (the Always Free allowance is 200 GB in total).
5. Networking: keep **assign a public IPv4 address** on.
6. Add your SSH public key (section 3) when the form asks for it.
7. Create it, and write down the **public IP address**.

---

## 3. Your SSH key, and signing in

On your laptop, if you do not already have a key:

```sh
ssh-keygen -t ed25519 -C "flybirds-admin"
cat ~/.ssh/id_ed25519.pub        # paste this into the cloud console
```

Then sign in to the machine (Oracle's Ubuntu images use the `ubuntu` user):

```sh
ssh ubuntu@<public-ip>
```

---

## 4. Open ports 80 and 443

This is the step people miss, because Oracle blocks traffic in **two** places.

**a) In the cloud console** — the subnet's security list (or network security group): add two **ingress** rules, source `0.0.0.0/0`, TCP, destination ports **80** and **443**.

**b) On the machine itself** — Oracle's Ubuntu images ship with iptables rules that drop everything except SSH:

```sh
sudo iptables -I INPUT 6 -m state --state NEW -p tcp --dport 80 -j ACCEPT
sudo iptables -I INPUT 6 -m state --state NEW -p tcp --dport 443 -j ACCEPT
sudo netfilter-persistent save
```

Leave SSH (22) open and open nothing else. The database and the backend publish no ports: only the TLS edge is reachable from the internet, and that is how it should stay.

Check from your laptop, not from the machine:

```sh
curl -sS -m 10 -o /dev/null -w '%{http_code}\n' http://<public-ip>/   # anything but a timeout is progress
```

---

## 5. Install Docker

```sh
sudo apt-get update && sudo apt-get install -y ca-certificates curl git
sudo install -m 0755 -d /etc/apt/keyrings
sudo curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
sudo chmod a+r /etc/apt/keyrings/docker.asc
echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo "$VERSION_CODENAME") stable" \
  | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
sudo apt-get update
sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
sudo usermod -aG docker "$USER"
```

Sign out and back in so the group takes effect, then check both pieces:

```sh
docker --version && docker compose version
systemctl is-enabled docker          # must print "enabled" — this is what restarts the school after a reboot
```

On a 1 GB machine, add swap before building images:

```sh
sudo fallocate -l 2G /swapfile && sudo chmod 600 /swapfile && sudo mkswap /swapfile && sudo swapon /swapfile
echo '/swapfile none swap sw 0 0' | sudo tee -a /etc/fstab
```

---

## 6. Get the code

```sh
sudo mkdir -p /opt/flybirds && sudo chown "$USER" /opt/flybirds
git clone https://github.com/mdrayhankvr-creator/Fly_Birds.git /opt/flybirds/app
cd /opt/flybirds/app
git checkout <the release tag or commit you are deploying>
```

Write down which commit it is. Section 11 needs the laptop and the server to be on the **same** one.

---

## 7. The address: a free subdomain now, your own domain later

Sign-in needs HTTPS. The session cookies are marked `Secure`, so over plain `http://` a password is accepted and then the session is lost. Both options below give real HTTPS; **do not plan to run on the bare IP address.**

### Option A — a free subdomain (no domain of your own)

1. Open [duckdns.org](https://www.duckdns.org), sign in with any of the offered accounts, and claim a name, for example `greenvalley`. You get `greenvalley.duckdns.org`.
2. Put the machine's public IP in the box and press **update ip**.
3. Check it from your laptop:

   ```sh
   dig +short greenvalley.duckdns.org      # must print your public IP
   ```

Let's Encrypt issues certificates for DuckDNS names, so HTTPS works straight away. Use `greenvalley.duckdns.org` as `SCHOOL_DOMAIN` in section 8.

### Option B — the school's own domain

Create an **A record** (and **AAAA** if the machine has IPv6) for `school.greenvalley.edu.bd` pointing at the public IP, wait for `dig +short` to show it, and use that name as `SCHOOL_DOMAIN`.

### Moving from A to B later

Nothing is lost, and no data is touched:

```sh
cd /opt/flybirds/app
sed -i 's/^SCHOOL_DOMAIN=.*/SCHOOL_DOMAIN=school.greenvalley.edu.bd/' deploy/greenvalley.env
sed -i 's#^PUBLIC_BASE_URL=.*#PUBLIC_BASE_URL=https://school.greenvalley.edu.bd#' deploy/greenvalley.env
docker compose -p greenvalley --env-file deploy/greenvalley.env -f deploy/compose.prod.yaml up -d
```

Caddy gets a certificate for the new name within a minute. Keep the old DNS record pointing at the machine for a day or two so anyone with the old address still arrives. `PUBLIC_BASE_URL` is what the QR code on a marksheet and the "forgot password" link point at, so change both lines together; marksheets printed before the change still carry the old address.

---

## 8. The school's settings file

On a paid server with a Go toolchain, `school-provision` writes this file for you ([DEPLOYMENT.md §3.2](DEPLOYMENT.md#32-provision-the-schools-configuration)). A free machine has no Go installed and does not need it: the file is four values and two secrets.

```sh
cd /opt/flybirds/app
cp deploy/school.env.example deploy/greenvalley.env
chmod 600 deploy/greenvalley.env
nano deploy/greenvalley.env
```

Set:

| Setting | Value |
|---|---|
| `COMPOSE_PROJECT_NAME` | `greenvalley` — a short code, no spaces |
| `COMPOSE_PROFILES` | `edge` — includes Caddy, which gets the certificate |
| `SCHOOL_DOMAIN` | the name from section 7 |
| `PUBLIC_BASE_URL` | `https://` + the same name |
| `ACME_EMAIL` | an address Let's Encrypt can warn about expiry |
| `POSTGRES_PASSWORD` | `openssl rand -hex 24` |
| `JWT_SIGNING_KEY_B64` | `openssl rand -base64 32` |
| `BACKUP_RETENTION` | `7` |
| `BACKUP_INTERVAL` | `0` — section 10 runs the backup from cron at a fixed hour instead |
| `PAYMENT_SIMULATION_ALLOWED` | `false`, always, for a real school |

Generate the two secrets straight into the file so they are never on screen or in your shell history:

```sh
printf 'POSTGRES_PASSWORD=%s\n' "$(openssl rand -hex 24)" >> deploy/greenvalley.env
printf 'JWT_SIGNING_KEY_B64=%s\n' "$(openssl rand -base64 32)" >> deploy/greenvalley.env
```

(then delete the empty `POSTGRES_PASSWORD=` and `JWT_SIGNING_KEY_B64=` lines the example file left behind.)

Start it:

```sh
docker compose -p greenvalley --env-file deploy/greenvalley.env -f deploy/compose.prod.yaml up -d --build
```

The first build takes 5–15 minutes on a free machine. Then:

```sh
docker compose -p greenvalley --env-file deploy/greenvalley.env -f deploy/compose.prod.yaml ps
curl -sS https://greenvalley.duckdns.org/api/readyz
```

`ps` must show `db`, `backend` (healthy), `web` and `caddy` running, and `readyz` must answer `{"data":{"status":"ready"}}`. If the certificate has not arrived, `docker compose ... logs caddy` says why — almost always port 80 (section 4) or DNS (section 7).

**The first administrator.** The stack starts with no accounts:

```sh
docker compose -p greenvalley --env-file deploy/greenvalley.env -f deploy/compose.prod.yaml \
  exec backend /app/create-user -email head@greenvalley.edu.bd -role admin
```

It asks for a password without echoing it. Do **not** run `seed-demo` on a real school: it creates demo accounts with known roles.

---

## 9. "Forgot password" email (optional)

Without this, the sign-in page keeps telling people to ask the school office, and an administrator resets passwords from **Admin → Users**. That is a perfectly good arrangement for a small school. With it, staff recover their own accounts.

1. Create a free [Brevo](https://www.brevo.com) account and open **SMTP & API → SMTP**. It gives you a login (your email) and an **SMTP key**.
2. Add to `deploy/greenvalley.env`:

   ```sh
   SMTP_HOST=smtp-relay.brevo.com
   SMTP_PORT=587
   SMTP_SECURITY=starttls
   SMTP_USERNAME=<the login Brevo shows>
   SMTP_PASSWORD=<the SMTP key>
   SMTP_FROM=noreply@greenvalley.edu.bd
   SMTP_FROM_NAME=Green Valley School
   ```

3. Restart the backend and check the log says it is on:

   ```sh
   docker compose -p greenvalley --env-file deploy/greenvalley.env -f deploy/compose.prod.yaml up -d
   docker compose -p greenvalley --env-file deploy/greenvalley.env -f deploy/compose.prod.yaml logs backend | grep -i "password reset email"
   ```

4. On the sign-in page, **Forgot your password?** now appears. Ask for a link with a real staff address and check the message arrives.

Notes worth knowing before you promise it to a school:

- `PUBLIC_BASE_URL` is required once `SMTP_HOST` is set; the backend refuses to start without it, because the emailed link needs an address.
- Mail from a brand-new sender often lands in spam until the domain has **SPF** and **DKIM** records. Brevo's console gives you both; add them at your registrar. Until then, tell people to check the spam folder.
- A link works **once** and expires after an hour. One address may ask for three links an hour; the web server limits how often any one client may ask.
- The page answers the same whether or not the address has an account, so it cannot be used to find out who has one.
- Students who sign in with a Student ID or a phone number have no email address on file, so this is for staff and for families who gave an address. The office still resets those.

---

## 10. A daily backup, and the restore you have tested

An archive holds the whole database and every uploaded file. The repository has a script that makes one, copies it out of the Docker volume onto the host disk, checks the copy, and keeps the newest seven.

```sh
sudo install -m 0755 /opt/flybirds/app/deploy/backup-daily.sh /usr/local/bin/flybirds-backup
sudo mkdir -p /opt/flybirds/backups && sudo chown "$USER" /opt/flybirds/backups

# Run it once by hand first. It prints what it did and fails loudly.
SCHOOL=greenvalley flybirds-backup
```

Then let cron do it every night at 02:15:

```sh
crontab -e
```

```cron
15 2 * * * SCHOOL=greenvalley /usr/local/bin/flybirds-backup >> /opt/flybirds/backup.log 2>&1
```

Cron emails you on any failure, and the log keeps one line per step. Check after the first night:

```sh
ls -lh /opt/flybirds/backups/greenvalley/ && tail -n 20 /opt/flybirds/backup.log
```

**A backup on the same machine as the data is not yet a backup.** Once a week, copy the newest archive somewhere else — from your laptop:

```sh
scp ubuntu@<public-ip>:/opt/flybirds/backups/greenvalley/'*.tar.gz' ~/flybirds-offsite/
```

### The restore command, and how to test it without risk

Restore into a **throwaway** project, never over the live school. The target must be at the same schema version as the backup, and `restore-backup` refuses a database that already has users unless you pass `-force`.

```sh
cd /opt/flybirds/app
# A disposable copy of the same release, on its own port, with its own volumes.
sed -e 's/^COMPOSE_PROJECT_NAME=.*/COMPOSE_PROJECT_NAME=restoretest/' \
    -e 's/^COMPOSE_PROFILES=.*/COMPOSE_PROFILES=/' \
    -e 's/^WEB_PORT=.*/WEB_PORT=8099/' deploy/greenvalley.env > deploy/restoretest.env
chmod 600 deploy/restoretest.env
docker compose -p restoretest --env-file deploy/restoretest.env -f deploy/compose.prod.yaml up -d --build

archive="$(ls -1t /opt/flybirds/backups/greenvalley/*.tar.gz | head -n 1)"
docker compose -p restoretest --env-file deploy/restoretest.env -f deploy/compose.prod.yaml \
  cp "$archive" backend:/app/backups/restore.tar.gz
docker compose -p restoretest --env-file deploy/restoretest.env -f deploy/compose.prod.yaml \
  exec -T backend /app/restore-backup -archive /app/backups/restore.tar.gz -uploads-dir /app/uploads -confirm
```

It prints how many tables, rows and attachments it loaded. Open `http://<public-ip>:8099` — nothing is published to the internet for this project — sign in with a real account from the backup, and look at a student and a marksheet. Then throw it away:

```sh
docker compose -p restoretest --env-file deploy/restoretest.env -f deploy/compose.prod.yaml down -v
rm deploy/restoretest.env
```

To restore over a live school after a real disaster, the same command with `-force` added, run against the school's own project. Everyone has to sign in again afterwards: refresh sessions are deliberately left out of backups.

---

## 11. Moving an existing school's data to the server

This is how the Jalalabad school's data, sitting in the `fbshow` stack on the laptop, gets to the server. It is the restore above with one extra rule.

**Both stacks must be on the same release.** A restore refuses an archive whose schema version differs from the target. So either deploy the server at the same commit the laptop stack runs, or update the laptop stack first and let it migrate.

On the laptop:

```sh
# 1. Make a fresh archive in the running stack and copy it out.
docker compose -p fbshow --env-file <fbshow env> -f <fbshow compose> exec -T backend /app/create-backup
#    It prints: <name> <bytes> <sha256>
docker compose -p fbshow --env-file <fbshow env> -f <fbshow compose> \
  exec -T backend cat /app/backups/<name> > jalalabad.tar.gz
sha256sum jalalabad.tar.gz        # must match the sha256 printed above

# 2. Send it to the server.
scp jalalabad.tar.gz ubuntu@<public-ip>:/tmp/
```

Older stacks have no `/app/create-backup`; there, sign in as an administrator and use **Admin → Backups → Create**, then **Download**.

On the server, into the school's own project, which so far has only the administrator you made in section 8:

```sh
cd /opt/flybirds/app
docker compose -p greenvalley --env-file deploy/greenvalley.env -f deploy/compose.prod.yaml \
  cp /tmp/jalalabad.tar.gz backend:/app/backups/incoming.tar.gz
docker compose -p greenvalley --env-file deploy/greenvalley.env -f deploy/compose.prod.yaml \
  exec -T backend /app/restore-backup -archive /app/backups/incoming.tar.gz -uploads-dir /app/uploads -confirm -force
rm /tmp/jalalabad.tar.gz
```

`-force` is needed because the administrator account already counts as a user. Afterwards:

- Every account from the laptop is there, **with the passwords it had there**. The administrator you created in section 8 is gone, replaced by the accounts from the archive — so make sure you know an administrator password from the laptop stack before you do this.
- Student and staff photos, the landing slideshow, the public page gallery and the result PDFs came across with the archive.
- Nobody is signed in: sessions are not backed up.
- School settings came across too, **including the school's name and logo** — but not the address: check **Admin → School settings** and the QR code on a marksheet, which now points at the server's `PUBLIC_BASE_URL`.
- Run section 12 before telling the school it is live.

---

## 12. The check after every deploy

```sh
sudo install -m 0755 /opt/flybirds/app/deploy/healthcheck.sh /usr/local/bin/flybirds-check
SCHOOL=greenvalley flybirds-check
```

It checks, in one command: the containers are up and the backend is healthy, the site answers over HTTPS with a valid certificate (and prints when it expires), the API answers, the database answers, the PDF renderer compiles a page in **English and Bangla**, and the disk has room. It prints one line per check and exits non-zero if any failed, so it also works as a cron job.

Keeping the machine tidy, once a month:

```sh
sudo apt-get update && sudo apt-get upgrade -y
docker builder prune -f           # the build cache is the biggest waster of disk
docker image prune -f             # untagged images only; tagged releases stay for rollback
SCHOOL=greenvalley flybirds-check
```

After a reboot nothing needs doing: `restart: unless-stopped` on every service plus an enabled Docker service brings the school back by itself. Test it once, early, so you know:

```sh
sudo reboot
# wait a minute, then from your laptop:
curl -sS https://greenvalley.duckdns.org/api/readyz
```

If you would rather not rely on the restart policy alone — for instance after a Docker upgrade that leaves containers stopped — add a boot unit as well:

```sh
sudo tee /etc/systemd/system/flybirds.service > /dev/null <<'UNIT'
[Unit]
Description=Fly_Birds school stack
Requires=docker.service
After=docker.service

[Service]
Type=oneshot
RemainAfterExit=yes
WorkingDirectory=/opt/flybirds/app
ExecStart=/usr/bin/docker compose -p greenvalley --env-file deploy/greenvalley.env -f deploy/compose.prod.yaml up -d
ExecStop=/usr/bin/docker compose -p greenvalley --env-file deploy/greenvalley.env -f deploy/compose.prod.yaml stop

[Install]
WantedBy=multi-user.target
UNIT
sudo systemctl daemon-reload && sudo systemctl enable --now flybirds.service
```

### When something is wrong

| What you see | Look at |
|---|---|
| The site times out | Ports 80/443 in **both** places (section 4) |
| "Not secure", or no certificate | `docker compose ... logs caddy`; DNS must resolve to this machine and port 80 must be open |
| Sign-in accepts the password then returns to sign-in | You are on `http://`. Secure cookies need HTTPS (section 7) |
| `readyz` answers 503 | `docker compose ... logs db backend`; usually the database is still starting |
| A marksheet says "The document could not be prepared" | `docker compose ... logs backend`; run `flybirds-check`, which compiles a test page |
| No reset email arrives | `docker compose ... logs backend | grep -i "password reset email"`; then SPF/DKIM and the spam folder (section 9) |
| The disk is filling up | `flybirds-check`, then `docker builder prune -f`; check `BACKUP_RETENTION` and the host copies in `/opt/flybirds/backups` |

---
---

# ১৩. বাংলা নির্দেশনা

নিচের অংশগুলোর নম্বর উপরের ইংরেজি অংশের সঙ্গে মেলে: ১ = 1, ২ = 2, এবং এভাবে। কমান্ডগুলো হুবহু একই, তাই দরকার হলে উপরের অংশ থেকে কপি করে নিতে পারেন।

## ১. যা আপনাকে নিজে করতে হবে

এগুলো আর কেউ আপনার হয়ে করে দিতে পারবে না। শুরুর আগেই তৈরি রাখুন।

| # | কী | কোথায় | খরচ |
|---|---|---|---|
| ১ | ফ্রি টিয়ার আছে এমন একটি ক্লাউড অ্যাকাউন্ট | [Oracle Cloud](https://www.oracle.com/cloud/free/) (পরামর্শ), বা Google Cloud | ফ্রি; Oracle পরিচয় যাচাইয়ের জন্য কার্ড চায়, কিন্তু Always Free সম্পদের জন্য টাকা কাটে না |
| ২ | একটি SSH কী জোড়া | নিজের ল্যাপটপে তৈরি (৩ নম্বর অংশ) | ফ্রি |
| ৩ | একটি ফ্রি সাবডোমেইন, **অথবা** নিজের ডোমেইন | ফ্রির জন্য [DuckDNS](https://www.duckdns.org), নয়তো আপনার রেজিস্ট্রার | ফ্রি / ডোমেইনের দাম |
| ৪ | "পাসওয়ার্ড ভুলে গেছেন" ইমেইলের জন্য একটি প্রোভাইডার অ্যাকাউন্ট | [Brevo](https://www.brevo.com) ফ্রি প্ল্যান, দিনে ৩০০ ইমেইল | ফ্রি; বাধ্যতামূলক নয় |
| ৫ | আপনার পছন্দের মান: স্কুলের কোড, স্কুলের ডোমেইন, সার্টিফিকেটের নোটিশের জন্য একটি ইমেইল | — | — |

Oracle-এর Always Free ARM মেশিন (**VM.Standard.A1.Flex**, সর্বোচ্চ ৪ কোর ও ২৪ GB) বেছে নিন: একটি স্কুলের যা দরকার তার চেয়ে অনেক বেশি, ১২ মাস পরে শেষ হয়ে যায় না, এবং এই স্ট্যাকের সব ইমেজ ARM-এর জন্যও তৈরি আছে। আপনার অঞ্চলে ARM না পাওয়া গেলে Always Free **VM.Standard.E2.1.Micro** (১ কোর, ১ GB)-তেও ছোট স্কুল চলে, তবে ২ GB swap দিন এবং ইমেজ বিল্ড ধীর হবে ধরে নিন।

**`deploy/<code>.env` কখনো Git-এ কমিট করবেন না।** এতে স্কুলের ডেটাবেস পাসওয়ার্ড ও সাইনিং কী থাকে। এটি আগে থেকেই `.gitignore`-এ আছে।

## ২. মেশিন তৈরি করুন

১. ক্লাউড কনসোলে একটি **Compute instance** তৈরি করুন।
২. ইমেজ: **Ubuntu 24.04 LTS** (A1 মেশিনের জন্য `aarch64`/ARM বিল্ড)।
৩. শেইপ: **VM.Standard.A1.Flex**; ২ কোর ও ৮ GB যথেষ্ট (৪ ও ২৪-ও ফ্রি)।
৪. বুট ভলিউম: **৫০ GB** (Always Free সীমা মোট ২০০ GB)।
৫. নেটওয়ার্ক: **assign a public IPv4 address** চালু রাখুন।
৬. ফর্ম যখন চাইবে, আপনার SSH পাবলিক কী দিন (৩ নম্বর অংশ)।
৭. তৈরি করুন, এবং **পাবলিক IP** লিখে রাখুন।

## ৩. SSH কী ও সাইন ইন

ল্যাপটপে, কী না থাকলে:

```sh
ssh-keygen -t ed25519 -C "flybirds-admin"
cat ~/.ssh/id_ed25519.pub        # এটি ক্লাউড কনসোলে পেস্ট করুন
```

তারপর মেশিনে ঢুকুন (Oracle-এর Ubuntu ইমেজে ব্যবহারকারী `ubuntu`):

```sh
ssh ubuntu@<public-ip>
```

## ৪. ৮০ ও ৪৪৩ পোর্ট খুলুন

এই ধাপটিই সবচেয়ে বেশি বাদ পড়ে, কারণ Oracle **দুই জায়গায়** ট্রাফিক আটকায়।

**ক) ক্লাউড কনসোলে** — সাবনেটের security list (বা network security group)-এ দুটি **ingress** রুল যোগ করুন: source `0.0.0.0/0`, TCP, destination port **৮০** ও **৪৪৩**।

**খ) মেশিনের ভিতরে** — Oracle-এর Ubuntu ইমেজে iptables রুল থাকে যা SSH ছাড়া সব ফেলে দেয়:

```sh
sudo iptables -I INPUT 6 -m state --state NEW -p tcp --dport 80 -j ACCEPT
sudo iptables -I INPUT 6 -m state --state NEW -p tcp --dport 443 -j ACCEPT
sudo netfilter-persistent save
```

SSH (২২) খোলা রাখুন, আর কিছু খুলবেন না। ডেটাবেস ও ব্যাকএন্ড কোনো পোর্ট প্রকাশ করে না: ইন্টারনেট থেকে শুধু TLS edge-এ পৌঁছানো যায়, এবং এটি এমনই থাকা উচিত।

মেশিন থেকে নয়, **ল্যাপটপ থেকে** পরীক্ষা করুন:

```sh
curl -sS -m 10 -o /dev/null -w '%{http_code}\n' http://<public-ip>/
```

## ৫. Docker ইনস্টল করুন

উপরের ৫ নম্বর অংশের কমান্ডগুলো হুবহু চালান। তারপর দুটো জিনিস মিলিয়ে নিন:

```sh
docker --version && docker compose version
systemctl is-enabled docker          # "enabled" লেখা থাকতে হবে — রিবুটের পরে স্কুল এটিই আবার চালু করে
```

গ্রুপ কাজ করতে সাইন আউট করে আবার ঢুকুন। ১ GB মেশিন হলে ইমেজ বিল্ডের আগে swap যোগ করুন (উপরের ৫ নম্বর অংশ)।

## ৬. কোড নিয়ে আসুন

```sh
sudo mkdir -p /opt/flybirds && sudo chown "$USER" /opt/flybirds
git clone https://github.com/mdrayhankvr-creator/Fly_Birds.git /opt/flybirds/app
cd /opt/flybirds/app
git checkout <যে রিলিজ ট্যাগ বা কমিট দিচ্ছেন>
```

কোন কমিট দিলেন লিখে রাখুন। ১১ নম্বর অংশে ল্যাপটপ ও সার্ভার **একই** কমিটে থাকা দরকার।

## ৭. ঠিকানা: আপাতত ফ্রি সাবডোমেইন, পরে নিজের ডোমেইন

সাইন ইনের জন্য HTTPS লাগে। সেশন কুকি `Secure` চিহ্নিত, তাই শুধু `http://`-তে পাসওয়ার্ড গ্রহণ হয় কিন্তু সেশন হারিয়ে যায়। নিচের দুটি পথেই আসল HTTPS পাওয়া যায়; **শুধু IP ঠিকানায় চালানোর পরিকল্পনা করবেন না।**

### পথ ক — ফ্রি সাবডোমেইন (নিজের ডোমেইন নেই)

১. [duckdns.org](https://www.duckdns.org) খুলে সাইন ইন করুন এবং একটি নাম নিন, যেমন `greenvalley`। আপনি পাবেন `greenvalley.duckdns.org`।
২. ঘরে মেশিনের পাবলিক IP বসিয়ে **update ip** চাপুন।
৩. ল্যাপটপ থেকে মিলিয়ে নিন: `dig +short greenvalley.duckdns.org` আপনার IP দেখাবে।

Let's Encrypt DuckDNS নামের জন্য সার্টিফিকেট দেয়, তাই HTTPS সঙ্গে সঙ্গেই কাজ করে।

### পথ খ — স্কুলের নিজের ডোমেইন

`school.greenvalley.edu.bd`-এর জন্য একটি **A রেকর্ড** (মেশিনে IPv6 থাকলে **AAAA**-ও) পাবলিক IP-তে বসান, `dig +short`-এ দেখা পর্যন্ত অপেক্ষা করুন, এবং ওই নামটিই `SCHOOL_DOMAIN` হিসেবে দিন।

### পরে ক থেকে খ-তে যাওয়া

কোনো ডেটা নষ্ট হয় না — উপরের ৭ নম্বর অংশের তিনটি কমান্ড চালান। এক মিনিটের মধ্যে Caddy নতুন নামের সার্টিফিকেট নিয়ে নেয়। পুরনো DNS রেকর্ড আরও এক-দুই দিন রেখে দিন, যাতে পুরনো ঠিকানা জানা লোকও পৌঁছাতে পারে। মার্কশিটের QR কোড ও পাসওয়ার্ড লিংক `PUBLIC_BASE_URL`-এ যায়, তাই দুটি লাইন একসঙ্গে বদলান; আগে ছাপা মার্কশিটে পুরনো ঠিকানাই থাকবে।

## ৮. স্কুলের সেটিংস ফাইল

```sh
cd /opt/flybirds/app
cp deploy/school.env.example deploy/greenvalley.env
chmod 600 deploy/greenvalley.env
nano deploy/greenvalley.env
```

যা বসাবেন: `COMPOSE_PROJECT_NAME=greenvalley`, `COMPOSE_PROFILES=edge`, `SCHOOL_DOMAIN` (৭ নম্বর অংশের নাম), `PUBLIC_BASE_URL=https://` + একই নাম, `ACME_EMAIL`, `BACKUP_RETENTION=7`, `BACKUP_INTERVAL=0` (১০ নম্বর অংশ নির্দিষ্ট সময়ে cron দিয়ে ব্যাকআপ নেবে), এবং আসল স্কুলের জন্য সব সময় `PAYMENT_SIMULATION_ALLOWED=false`।

দুটি গোপন মান সরাসরি ফাইলে তৈরি করুন, যাতে পর্দায় বা শেল ইতিহাসে না থাকে:

```sh
printf 'POSTGRES_PASSWORD=%s\n' "$(openssl rand -hex 24)" >> deploy/greenvalley.env
printf 'JWT_SIGNING_KEY_B64=%s\n' "$(openssl rand -base64 32)" >> deploy/greenvalley.env
```

(তারপর উদাহরণ ফাইলের খালি `POSTGRES_PASSWORD=` ও `JWT_SIGNING_KEY_B64=` লাইনগুলো মুছে দিন।)

চালু করুন:

```sh
docker compose -p greenvalley --env-file deploy/greenvalley.env -f deploy/compose.prod.yaml up -d --build
```

ফ্রি মেশিনে প্রথম বিল্ডে ৫–১৫ মিনিট লাগে। তারপর `ps`-এ `db`, `backend` (healthy), `web` ও `caddy` চলতে দেখবেন, এবং `curl -sS https://<আপনার নাম>/api/readyz` উত্তর দেবে `{"data":{"status":"ready"}}`। সার্টিফিকেট না এলে `docker compose ... logs caddy` কারণ বলে দেয় — প্রায় সব সময়ই ৮০ পোর্ট (৪ নম্বর) বা DNS (৭ নম্বর)।

**প্রথম প্রশাসক।** স্ট্যাক শুরু হয় কোনো অ্যাকাউন্ট ছাড়া:

```sh
docker compose -p greenvalley --env-file deploy/greenvalley.env -f deploy/compose.prod.yaml \
  exec backend /app/create-user -email head@greenvalley.edu.bd -role admin
```

এটি পাসওয়ার্ড চাইবে, পর্দায় দেখাবে না। আসল স্কুলে `seed-demo` **চালাবেন না**: এটি পরিচিত ভূমিকার ডেমো অ্যাকাউন্ট বানায়।

## ৯. "পাসওয়ার্ড ভুলে গেছেন" ইমেইল (বাধ্যতামূলক নয়)

এটি না করলে সাইন ইন পাতা আগের মতোই স্কুল অফিসে যোগাযোগ করতে বলবে, এবং প্রশাসক **Admin → Users** থেকে পাসওয়ার্ড বদলে দেবেন। ছোট স্কুলের জন্য এটিই যথেষ্ট। করলে শিক্ষক-কর্মচারীরা নিজেরাই অ্যাকাউন্ট ফিরে পাবেন।

১. ফ্রি [Brevo](https://www.brevo.com) অ্যাকাউন্ট খুলে **SMTP & API → SMTP** দেখুন। সেখানে একটি লগইন (আপনার ইমেইল) ও একটি **SMTP key** পাবেন।
২. `deploy/greenvalley.env`-এ উপরের ৯ নম্বর অংশের সাত লাইন যোগ করুন।
৩. ব্যাকএন্ড আবার চালু করে লগে দেখে নিন: `logs backend | grep -i "password reset email"`।
৪. সাইন ইন পাতায় এখন **পাসওয়ার্ড ভুলে গেছেন?** দেখা যাবে। একটি আসল ঠিকানা দিয়ে লিংক চেয়ে দেখুন ইমেইল পৌঁছায় কি না।

স্কুলকে কথা দেওয়ার আগে যা জানা দরকার:

- `SMTP_HOST` দিলে `PUBLIC_BASE_URL` দেওয়া বাধ্যতামূলক; না দিলে ব্যাকএন্ড চালু হবে না, কারণ ইমেইলের লিংকের একটি ঠিকানা লাগে।
- নতুন প্রেরকের ইমেইল প্রায়ই স্পামে যায়, যতক্ষণ ডোমেইনে **SPF** ও **DKIM** রেকর্ড না থাকে। Brevo কনসোল দুটোই দেয়; রেজিস্ট্রারে বসিয়ে দিন। তার আগে সবাইকে স্পাম ফোল্ডার দেখতে বলুন।
- একটি লিংক **একবারই** কাজ করে এবং এক ঘণ্টা পরে অচল হয়। এক ঠিকানা ঘণ্টায় তিনটি লিংক চাইতে পারে; এক ক্লায়েন্ট কতবার চাইতে পারে তা ওয়েব সার্ভার সীমিত করে।
- ঠিকানাটির অ্যাকাউন্ট থাকুক বা না থাকুক পাতা একই উত্তর দেয়, তাই এটি দিয়ে কার অ্যাকাউন্ট আছে জানা যায় না।
- স্টুডেন্ট আইডি বা মোবাইল নম্বর দিয়ে সাইন ইন করা শিক্ষার্থীদের ইমেইল ঠিকানা নথিতে নেই, তাই এটি শিক্ষক-কর্মচারী ও যেসব পরিবার ঠিকানা দিয়েছে তাদের জন্য। বাকিদের পাসওয়ার্ড অফিসই বদলে দেবে।

## ১০. প্রতিদিনের ব্যাকআপ, এবং পরীক্ষা করা রিস্টোর

একটি আর্কাইভে পুরো ডেটাবেস ও আপলোড করা সব ফাইল থাকে। রিপোজিটরিতে এমন একটি স্ক্রিপ্ট আছে যা আর্কাইভ বানায়, Docker ভলিউম থেকে হোস্ট ডিস্কে কপি করে, কপিটি যাচাই করে, এবং সবশেষ সাতটি রাখে।

```sh
sudo install -m 0755 /opt/flybirds/app/deploy/backup-daily.sh /usr/local/bin/flybirds-backup
sudo mkdir -p /opt/flybirds/backups && sudo chown "$USER" /opt/flybirds/backups
SCHOOL=greenvalley flybirds-backup        # আগে একবার হাতে চালান
```

তারপর প্রতি রাত ২:১৫-তে cron চালাবে:

```cron
15 2 * * * SCHOOL=greenvalley /usr/local/bin/flybirds-backup >> /opt/flybirds/backup.log 2>&1
```

কোনো ব্যর্থতা হলে cron আপনাকে ইমেইল করে, আর লগে প্রতিটি ধাপের একটি লাইন থাকে। প্রথম রাতের পরে `ls -lh /opt/flybirds/backups/greenvalley/` ও `tail -n 20 /opt/flybirds/backup.log` দেখে নিন।

**যে মেশিনে ডেটা, সেই মেশিনের ব্যাকআপ এখনো ব্যাকআপ নয়।** সপ্তাহে একবার সবশেষ আর্কাইভটি অন্য কোথাও কপি করুন (উপরের ১০ নম্বর অংশের `scp`)।

### রিস্টোর কমান্ড, এবং ঝুঁকি ছাড়া তা পরীক্ষা করা

রিস্টোর সব সময় একটি **ফেলে দেওয়ার** প্রজেক্টে করুন, কখনো চালু স্কুলের উপরে নয়। লক্ষ্য ডেটাবেস আর্কাইভের মতো একই স্কিমা সংস্করণে থাকতে হবে, এবং যে ডেটাবেসে ইতিমধ্যেই ব্যবহারকারী আছে সেখানে `-force` ছাড়া `restore-backup` কাজ করবে না। কমান্ডগুলো উপরের ১০ নম্বর অংশে হুবহু দেওয়া আছে: একটি আলাদা `restoretest` প্রজেক্ট তৈরি করুন, আর্কাইভ কপি করুন, `restore-backup` চালান, `http://<public-ip>:8099`-এ ঢুকে একজন শিক্ষার্থী ও একটি মার্কশিট দেখুন, তারপর `down -v` দিয়ে মুছে ফেলুন।

সত্যিকারের বিপর্যয়ের পরে চালু স্কুলের উপরে রিস্টোর করতে একই কমান্ড, সঙ্গে `-force`। এরপর সবাইকে আবার সাইন ইন করতে হবে: রিফ্রেশ সেশন ইচ্ছে করেই ব্যাকআপে রাখা হয় না।

## ১১. চালু স্কুলের ডেটা সার্ভারে নেওয়া

ল্যাপটপের `fbshow` স্ট্যাকে থাকা জালালাবাদ স্কুলের ডেটা এভাবেই সার্ভারে যাবে। উপরের রিস্টোরই, সঙ্গে একটি বাড়তি নিয়ম।

**দুই স্ট্যাক একই রিলিজে থাকতে হবে।** আর্কাইভের স্কিমা সংস্করণ লক্ষ্যের থেকে আলাদা হলে রিস্টোর হবে না। তাই হয় ল্যাপটপ যে কমিটে চলছে সার্ভারেও সেটিই দিন, নয়তো আগে ল্যাপটপের স্ট্যাক হালনাগাদ করে মাইগ্রেট হতে দিন।

ল্যাপটপে: `exec -T backend /app/create-backup` দিয়ে নতুন আর্কাইভ বানান (এটি নাম, আকার ও sha256 ছাপে), `exec -T backend cat /app/backups/<name> > jalalabad.tar.gz` দিয়ে বাইরে আনুন, `sha256sum` মিলিয়ে নিন, তারপর `scp` দিয়ে সার্ভারে পাঠান। পুরনো স্ট্যাকে `/app/create-backup` নেই; সেখানে প্রশাসক হিসেবে ঢুকে **Admin → Backups → Create**, তারপর **Download** ব্যবহার করুন।

সার্ভারে, স্কুলের নিজের প্রজেক্টে (যেখানে এখন শুধু ৮ নম্বর অংশে বানানো প্রশাসক আছে), উপরের ১১ নম্বর অংশের তিনটি কমান্ড চালান। `-force` লাগবে, কারণ ওই প্রশাসক অ্যাকাউন্টটিও একজন ব্যবহারকারী হিসেবে গোনা হয়। এরপর:

- ল্যাপটপের সব অ্যাকাউন্ট **সেখানকার পাসওয়ার্ড নিয়েই** চলে আসবে। ৮ নম্বর অংশে বানানো প্রশাসক আর থাকবে না, আর্কাইভের অ্যাকাউন্টগুলোই বসবে — তাই এটি করার আগে ল্যাপটপের একজন প্রশাসকের পাসওয়ার্ড জানা আছে কি না নিশ্চিত হয়ে নিন।
- শিক্ষার্থী ও কর্মচারীর ছবি, হোমপেজের স্লাইডশো, পাবলিক পাতার গ্যালারি এবং ফলাফলের PDF আর্কাইভের সঙ্গেই এসেছে।
- কেউ সাইন ইন করা থাকবে না: সেশন ব্যাকআপে থাকে না।
- স্কুল সেটিংসও এসেছে, **স্কুলের নাম ও লোগোসহ** — কিন্তু ঠিকানা নয়: **Admin → School settings** এবং মার্কশিটের QR কোড দেখে নিন, যা এখন সার্ভারের `PUBLIC_BASE_URL`-এ যাবে।
- স্কুলকে "লাইভ" বলার আগে ১২ নম্বর অংশ চালান।

## ১২. প্রতিটি ডিপ্লয়ের পরের পরীক্ষা

```sh
sudo install -m 0755 /opt/flybirds/app/deploy/healthcheck.sh /usr/local/bin/flybirds-check
SCHOOL=greenvalley flybirds-check
```

একটি কমান্ডেই দেখে নেয়: কন্টেইনার চালু ও ব্যাকএন্ড healthy কি না, সাইট HTTPS-এ বৈধ সার্টিফিকেট নিয়ে উত্তর দেয় কি না (এবং কত দিন সার্টিফিকেট আছে), API উত্তর দেয় কি না, ডেটাবেস উত্তর দেয় কি না, PDF রেন্ডারার **ইংরেজি ও বাংলা** একটি পাতা বানাতে পারে কি না, এবং ডিস্কে জায়গা আছে কি না। প্রতিটি পরীক্ষার একটি লাইন ছাপে, এবং কোনোটি ব্যর্থ হলে শূন্য ছাড়া অন্য কোড দিয়ে শেষ হয় — তাই cron-এও চলে।

মাসে একবার মেশিন পরিষ্কার রাখতে: `sudo apt-get update && sudo apt-get upgrade -y`, `docker builder prune -f` (বিল্ড ক্যাশই সবচেয়ে বেশি ডিস্ক খায়), `docker image prune -f` (শুধু ট্যাগহীন ইমেজ; রোলব্যাকের জন্য ট্যাগ করা রিলিজ থেকে যায়), তারপর আবার `flybirds-check`।

রিবুটের পরে কিছু করতে হয় না: প্রতিটি সার্ভিসে `restart: unless-stopped` আর চালু করা Docker সার্ভিস স্কুলকে নিজে থেকেই ফিরিয়ে আনে। একবার, শুরুতেই, `sudo reboot` দিয়ে পরীক্ষা করে নিন এবং এক মিনিট পরে ল্যাপটপ থেকে `curl -sS https://<আপনার নাম>/api/readyz` দেখুন। শুধু restart নীতিতে ভরসা না করতে চাইলে উপরের ১২ নম্বর অংশের systemd ইউনিটটিও যোগ করে নিন।

### কিছু ভুল হলে

| যা দেখছেন | যা দেখবেন |
|---|---|
| সাইট সময় শেষ হয়ে যায় (timeout) | **দুই জায়গাতেই** ৮০/৪৪৩ পোর্ট (৪ নম্বর অংশ) |
| "Not secure", বা সার্টিফিকেট নেই | `logs caddy`; DNS এই মেশিনে আসতে হবে এবং ৮০ পোর্ট খোলা থাকতে হবে |
| পাসওয়ার্ড নেয়, তারপর আবার সাইন ইন পাতায় ফেরে | আপনি `http://`-তে আছেন। Secure কুকির জন্য HTTPS দরকার (৭ নম্বর অংশ) |
| `readyz` ৫০৩ বলে | `logs db backend`; সাধারণত ডেটাবেস তখনো চালু হচ্ছে |
| মার্কশিট বলে "The document could not be prepared" | `logs backend`; `flybirds-check` চালান, এটি একটি পরীক্ষামূলক পাতা বানায় |
| রিসেট ইমেইল আসে না | `logs backend | grep -i "password reset email"`; তারপর SPF/DKIM ও স্পাম ফোল্ডার (৯ নম্বর অংশ) |
| ডিস্ক ভরে যাচ্ছে | `flybirds-check`, তারপর `docker builder prune -f`; `BACKUP_RETENTION` ও `/opt/flybirds/backups`-এর কপিগুলো দেখুন |
