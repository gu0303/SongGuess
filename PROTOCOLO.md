# Song Guess — Protocolo WebSocket

Complementa o `REQUISITOS.md`. Os IDs entre parênteses (RD-05, M1-06…) apontam para os requisitos.

---

## 1. Decisões técnicas que afetam o protocolo

### 1.1 Áudio passa pelo servidor (proxy), tocado com Web Audio

O navegador **não** recebe a URL do Deezer. O servidor baixa a prévia e serve em `GET /audio/{token}`, no mesmo domínio do jogo. Motivos:

- **Volume no iPhone (RD-02):** o Safari do iOS ignora `audio.volume`. A saída é tocar pelo Web Audio com um `GainNode`.
- **CORS:** para o Web Audio processar o áudio, o arquivo precisa vir com cabeçalho CORS, e o CDN do Deezer não envia esse cabeçalho. Servindo pelo próprio servidor, a origem é a mesma e o problema desaparece.
- **Sincronia e duração do trecho (RD-00, CF-01):** o cliente baixa o arquivo inteiro, decodifica (`decodeAudioData`) e toca com `AudioBufferSourceNode.start(quando, 0, clipSeconds)`. O início e o corte em N segundos ficam precisos, e o replay (RD-03) é só tocar o mesmo buffer de novo.
- **Anti-trapaça (RG-04):** o `token` é aleatório por rodada e não revela nada da música.

Custo: cada prévia tem ~0,5 MB e sai do upload da sua internet uma vez por jogador por rodada. Com 6 jogadores dá ~3 MB por rodada, tranquilo.

Um único `AudioContext` por página, criado/retomado (`resume()`) no clique de "Estou pronto" (LB-02). Isso libera o áudio (RD-00).

### 1.2 Como o servidor sabe quem é o host

Pelo Cloudflare Tunnel, **todas as conexões chegam de `127.0.0.1`**, inclusive as dos amigos, porque o `cloudflared` roda na sua máquina. Então **não dá para identificar o host pelo IP**.

Solução: quem completa o login OAuth do Spotify (`/callback`) recebe um cookie `host_session` (HttpOnly, aleatório). No upgrade do WebSocket, o servidor confere esse cookie.

### 1.3 Uma sala por servidor

O servidor tem uma sala só, e o link do jogo é a própria URL do túnel. Simplifica tudo e basta para um grupo de amigos.

### 1.4 Relógio

O servidor é a única fonte de tempo:

- Os horários vão em **milissegundos Unix do servidor** (`startsAt`, `endsAt`).
- O cliente calcula o próprio desvio com `ping`/`pong` (ver 3.1) e converte para o relógio local e para o `AudioContext.currentTime`.
- **A pontuação por velocidade usa o horário em que o servidor recebeu o palpite:** `t = recebido - startsAt`. O cliente nunca informa o próprio tempo, porque poderia mentir.

### 1.5 O que nunca vai para o cliente antes do fim da rodada

Título, artista, capa, ID do Spotify, ISRC e URL do Deezer. No modo 1, as opções levam **IDs aleatórios por rodada** (`"o3"`), não IDs do Spotify, que poderiam ser consultados na API.

---

## 2. Formato das mensagens

JSON, um objeto por mensagem WebSocket:

```json
{ "type": "nome_do_evento", "data": { } }
```

- **C→S**: cliente para servidor. **S→C**: servidor para cliente.
- **broadcast**: vai para todos os jogadores. **privado**: só para um jogador.
- Mensagens com 🔒 são aceitas só do host.

---

## 3. Conexão

### 3.1 Entrada e reconexão

**C→S `hello`**, primeira mensagem após abrir `/ws`:

```json
{ "type": "hello", "data": { "name": "Ana", "token": null } }
```

`token` fica `null` na primeira vez. Na reconexão, o cliente manda o token que recebeu no `welcome` (guardado no `sessionStorage`).

**S→C `welcome`** (privado):

```json
{ "type": "welcome", "data": {
  "playerId": "p7",
  "token": "b41f…",
  "isHost": false,
  "serverTime": 1790000000000,
  "state": { "...": "ver room_state" }
}}
```

**S→C `room_state`** (privado, no `welcome` e sempre que o cliente precisar se ressincronizar): o estado inteiro da sala.

```json
{ "type": "room_state", "data": {
  "phase": "lobby",
  "config": { "mode": 1, "playlistId": "…", "playlistName": "Rock 80s",
              "clipSeconds": 10, "rounds": 20, "usableTracks": 57 },
  "players": [
    { "id": "p1", "name": "Gustavo", "isHost": true, "ready": true,
      "connected": true, "score": 0, "correctSongs": 0 }
  ],
  "round": null
}}
```

`phase`: `lobby` · `loading` (preparando rodada) · `playing` · `reveal` (mostrando resposta) · `podium`.

Durante `playing`, o campo `round` traz `number`, `startsAt`, `endsAt`, `audio`, `options` (modo 1) e o status do próprio jogador na rodada.

**Reconexão:** se o jogador cair, ele **sai da contagem da rodada na hora** (RD-06), e os outros recebem `players`. Se voltar com o mesmo `token`, recupera nome, pontos e o status da rodada atual. Se ainda não tinha terminado a rodada, volta a jogar.

**Relógio:** C→S `ping` / S→C `pong` (privado).

```json
{ "type": "ping", "data": { "c": 1790000000123 } }
{ "type": "pong", "data": { "c": 1790000000123, "s": 1790000000180 } }
```

`desvio = s - (c + agora) / 2`. O cliente faz 5 pings ao entrar e usa a mediana. Depois, um ping a cada 30 s.

**S→C `error`** (privado):

```json
{ "type": "error", "data": { "code": "NOT_ALL_READY", "message": "Nem todos estão prontos" } }
```

---

## 4. Lobby

| Mensagem | Direção | Dados | Regra |
|---|---|---|---|
| `set_ready` | C→S | `{ "ready": true }` | LB-02. O cliente só envia depois de `AudioContext.resume()` dar certo |
| `players` | S→C broadcast | `{ "players": [ … ] }` | Lista completa, enviada a cada entrada, saída, pronto ou mudança de pontos |
| `set_config` 🔒 | C→S | `{ "mode": 2, "playlistId": "…", "clipSeconds": 15, "rounds": 30 }` | CF-01, CF-02. Validação: `clipSeconds` ∈ {5,10,15,20,30}, `rounds` 10–50 |
| `playlist_loading` | S→C broadcast | `{ "done": 40, "total": 120 }` | Progresso da checagem de prévias no Deezer |
| `config` | S→C broadcast | `{ …config, "usableTracks": 57, "maxRounds": 50 }` | Se `usableTracks` < rodadas, `rounds` é reduzido e o host é avisado (CF-03) |
| `start_game` 🔒 | C→S | `{}` | Recusado com `NOT_ALL_READY` se alguém não estiver pronto (LB-03, LB-04) |

**A lista de playlists** do host vem por HTTP, não pelo WebSocket: `GET /api/playlists` (exige o cookie `host_session`), que chama `GET /me/playlists` do Spotify.

**Checagem de prévias:** ao escolher a playlist, o servidor busca cada faixa no Deezer por ISRC e guarda só o **ID da faixa no Deezer**, não a URL da prévia, que expira. A URL é buscada de novo na hora da rodada. O Deezer limita a ~50 requisições a cada 5 s, então a checagem de playlists grandes leva alguns segundos (daí o `playlist_loading`).

---

## 5. Rodada

### 5.1 Sequência

```
servidor                                   clientes
   │── round_prepare (audio, opções) ──────▶│  baixam e decodificam o áudio
   │◀──────────────── audio_loaded ─────────│
   │   (espera todos ou 5 s)                │
   │── round_start (startsAt, endsAt) ─────▶│  tocam em startsAt
   │◀── choose / guess / vote_reveal ───────│
   │── guess_result (privado) ─────────────▶│
   │── player_status (broadcast) ──────────▶│
   │   … até todos terminarem ou endsAt …   │
   │── round_end (resposta + placar) ──────▶│
   │   (intervalo)                          │
   │── round_prepare (próxima) ────────────▶│
```

### 5.2 Mensagens

**S→C `round_prepare`** (broadcast):

```json
{ "type": "round_prepare", "data": {
  "round": 3, "totalRounds": 20,
  "audio": "/audio/9f2c7e1a…",
  "clipSeconds": 10,
  "options": [
    { "id": "o1", "label": "Iron Man — Black Sabbath" },
    { "id": "o2", "label": "Paranoid — Black Sabbath" },
    { "id": "o3", "label": "Zombie — The Cranberries" },
    { "id": "o4", "label": "Creep — Radiohead" }
  ]
}}
```

`options` só existe no modo 1. As opções vêm embaralhadas.

**C→S `audio_loaded`**: `{ "round": 3 }`. Se o download falhar, o cliente manda `{ "round": 3, "failed": true }` e o servidor segue sem esperar.

**S→C `round_start`** (broadcast), enviado quando todos carregaram ou após 5 s:

```json
{ "type": "round_start", "data": { "round": 3, "startsAt": 1790000003000, "endsAt": 1790000123000 } }
```

`startsAt` fica ~1,5 s no futuro para dar tempo de todos agendarem o início (RD-00). `endsAt = startsAt + 120 s` (RD-01).

**Replay (RD-03) e volume (RD-02)** são locais. Não geram mensagem.

### 5.3 Modo 1 — escolha

**C→S `choose`**:

```json
{ "type": "choose", "data": { "round": 3, "optionId": "o1" } }
```

**S→C `guess_result`** (privado):

```json
{ "type": "guess_result", "data": {
  "round": 3, "result": "band", "optionId": "o1",
  "roundPoints": 370, "finished": false
}}
```

| `result` | Quando | Pontos | `finished` |
|---|---|---|---|
| `song` | opção correta | `max(pontos_banda, P(t))` (M1-04, M1-06) | `true` |
| `band` | banda certa, música errada | `40% × P(t)` (M1-05); a opção fica bloqueada | `false` |
| `wrong` | banda e música erradas | 0, mantém o que já tinha (M1-08) | `true` |

O servidor recusa `choose` de quem já terminou ou de uma opção já bloqueada.

### 5.4 Modo 2 — digitação

**C→S `guess`**:

```json
{ "type": "guess", "data": { "round": 3, "text": "paranoid black sabath" } }
```

Limite: 1 palpite a cada 500 ms e no máximo 100 caracteres. Excessos são ignorados.

**S→C `guess_result`** (privado):

```json
{ "type": "guess_result", "data": {
  "round": 3, "attempt": 4, "percent": 96, "result": "song",
  "roundPoints": 786, "finished": true
}}
```

`result`: `song` (≥95% em título, título+banda ou banda+título), `band` (≥95% na banda, só na primeira vez) ou `none`. Pontuação conforme M2-08/M2-09.

### 5.5 Status visível para todos

**S→C `player_status`** (broadcast), a cada tentativa ou mudança de estado:

```json
{ "type": "player_status", "data": { "playerId": "p7", "percent": 81, "state": "band" } }
```

- `percent`: só no modo 2 (M2-03). O texto digitado nunca é enviado.
- `state`: `playing` · `band` ("acertou a banda") · `song` ("acertou a música") · `out` (errou no modo 1) · `gave_up` (apertou revelar).

### 5.6 Revelar

**C→S `vote_reveal`**: `{ "round": 3 }`. O jogador desiste (RD-05): fica `gave_up` e não pode mais tentar.

**S→C `reveal_votes`** (broadcast): `{ "votes": 2, "needed": 3 }`.

`needed` = jogadores conectados que ainda estavam jogando no início da votação. Na prática, **a rodada termina quando não sobra ninguém com `state` = `playing` ou `band`** (RD-06, RD-07), então quem desiste, acerta, erra ou cai reduz a contagem do mesmo jeito.

### 5.7 Fim da rodada

**S→C `round_end`** (broadcast), quando todos terminaram ou em `endsAt` (RD-07, RD-08):

```json
{ "type": "round_end", "data": {
  "round": 3,
  "answer": { "title": "Paranoid", "artist": "Black Sabbath",
              "cover": "https://i.scdn.co/image/…",
              "spotifyUrl": "https://open.spotify.com/track/…" },
  "results": [
    { "playerId": "p7", "state": "song", "roundPoints": 775 },
    { "playerId": "p2", "state": "band", "roundPoints": 370 }
  ],
  "scoreboard": [
    { "playerId": "p7", "name": "Ana", "score": 2410, "correctSongs": 3, "position": 1 },
    { "playerId": "p2", "name": "Gustavo", "score": 1980, "correctSongs": 2, "position": 2 }
  ],
  "nextRoundAt": 1790000131000
}}
```

`scoreboard` já vem ordenado e com posição calculada, com o desempate do PL-06 (mais músicas certas; empate total repete a posição). O cliente só desenha a coluna (PL-01, PL-02).

---

## 6. Fim da partida

Depois do `round_end` da última rodada, o servidor conduz o pódio (PL-03, PL-05). Quem controla o ritmo é o servidor, para todos verem juntos.

**S→C `podium_step`** (broadcast), uma a cada 3 s:

```json
{ "type": "podium_step", "data": { "position": 3, "players": [ { "id": "p4", "name": "Leo", "score": 1500 } ] } }
```

Ordem: posição 3, depois 2, depois 1. `players` é uma lista porque empates dividem a posição (PL-06). Com menos de 3 jogadores, as posições inexistentes são puladas (PL-04).

**S→C `podium_done`** (broadcast): `{ "scoreboard": [ … ] }`, com o placar final completo.

**C→S `back_to_lobby` 🔒**: todos voltam ao lobby, os pontos zeram, o `ready` de todos volta a `false` e a config fica editável (PL-07). O servidor manda `room_state` para todos.

---

## 7. Máquina de estados da sala

```
lobby ──start_game──▶ loading ──todos carregaram / 5 s──▶ playing
  ▲                      ▲                                   │
  │                      └──────── intervalo ────────── reveal ◀── todos terminaram / endsAt
  │                                                          │
  └────────── back_to_lobby ────────── podium ◀── última rodada
```

---

## 8. Códigos de erro

| Código | Quando |
|---|---|
| `NOT_HOST` | Mensagem 🔒 enviada por quem não é host |
| `NOT_ALL_READY` | `start_game` com alguém não pronto |
| `INVALID_CONFIG` | `clipSeconds`/`rounds` fora dos valores permitidos |
| `NOT_ENOUGH_TRACKS` | Playlist com menos de 10 músicas com prévia, ou menos de 4 no modo 1 |
| `WRONG_PHASE` | Mensagem que não cabe na fase atual (ex.: `choose` no lobby) |
| `ROUND_FINISHED` | Palpite de quem já terminou a rodada |
| `NAME_TAKEN` | Apelido já em uso na sala |
| `SPOTIFY_AUTH` | Token do Spotify expirado. O host precisa logar de novo |
