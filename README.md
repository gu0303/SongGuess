# 🎵 Song Guess

Jogo multiplayer de adivinhar músicas para jogar com amigos pelo navegador. O host escolhe uma playlist do Spotify, o servidor roda no próprio computador dele e os amigos entram por um link, sem instalar nada e sem precisar de conta no Spotify.

> **Status:** em desenvolvimento. Requisitos e protocolo definidos; implementação em andamento.

---

## Como funciona

1. O host faz login com a conta do Spotify e escolhe uma playlist, o modo de jogo e as configurações da partida.
2. Os amigos abrem o link, escolhem um apelido e clicam em **"Estou pronto"**.
3. Cada rodada toca um trecho da música ao mesmo tempo para todos. Quem adivinhar mais rápido ganha mais pontos.
4. No fim, um pódio revela o 3º, o 2º e o 1º lugar.

## Modos de jogo

### Modo 1 — Múltipla escolha
Quatro opções no formato **"Música — Banda"**, sorteadas da mesma playlist.

- Acertar a música vale os pontos cheios. Quanto mais rápido, mais pontos.
- Acertar só a banda (opção da banda certa, música errada) vale 40% e permite tentar de novo.
- Errar banda e música encerra a rodada para o jogador.

### Modo 2 — Digitação
O jogador digita o título, a banda ou os dois.

- A cada tentativa aparece a porcentagem de semelhança com a resposta, e os outros jogadores veem as porcentagens em tempo real.
- Conta como acerto a partir de **95%**. Não precisa do nome exato: trechos como `(Remastered)`, `- Ao Vivo` e `feat.` são ignorados.
- A velocidade pesa mais na pontuação. Cada tentativa extra desconta 5%.

## Regras gerais

| | |
|---|---|
| Duração do trecho | 5, 10, 15, 20 ou 30 segundos (definido pelo host) |
| Número de rodadas | 10 a 50 (definido pelo host) |
| Tempo por rodada | 2 minutos |
| Pontuação | de 1000 (resposta instantânea) a 100 (fim do tempo), linear |
| Revelar | quem aperta desiste da rodada; ela acaba quando ninguém mais está tentando |
| Desempate | quem acertou mais músicas; empate total divide a posição |

Cada jogador controla o próprio volume e pode repetir o trecho quantas vezes quiser, só para si.

Todas as regras estão detalhadas em [`REQUISITOS.md`](REQUISITOS.md).

---

## Arquitetura

```
 Amigos (navegador)                    Computador do host
 ┌──────────────┐                ┌────────────────────────────────┐
 │  HTML + JS   │◀── WebSocket ──│  song-guess.exe (Go)           │
 │  Web Audio   │◀── /audio ─────│   ├─ estado das salas/rodadas  │
 └──────────────┘       ▲        │   ├─ Spotify API (playlists)   │
                        │        │   └─ Deezer/iTunes (prévias)   │
              Cloudflare Tunnel ─┤                                │
                                 └────────────────────────────────┘
```

| Parte | Tecnologia |
|---|---|
| Servidor | Go (`net/http` da biblioteca padrão) |
| Tempo real | WebSocket com [`coder/websocket`](https://github.com/coder/websocket) |
| Frontend | HTML, CSS e JavaScript simples, embutidos no executável com `go:embed` |
| Playlists e metadados | Spotify Web API (OAuth, só o host faz login) |
| Áudio | Prévias de 30 s do Deezer (busca por ISRC), com o iTunes como alternativa |
| Acesso externo | Cloudflare Tunnel |

### Por que o áudio não vem do Spotify

Desde novembro de 2024, a API do Spotify não entrega mais prévias de áudio para apps novos. O Spotify fornece a lista de músicas e o **ISRC** (código internacional da gravação), e o servidor usa esse código para achar a mesma gravação no Deezer.

O áudio é repassado pelo próprio servidor em vez de o navegador baixar direto do Deezer. Assim o controle de volume funciona no iPhone e o início das músicas fica sincronizado entre os jogadores. Detalhes em [`PROTOCOLO.md`](PROTOCOLO.md).

---

## Requisitos

- Windows 11 (também compila para Linux e macOS)
- [Go](https://go.dev/) 1.22 ou superior
- [cloudflared](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/)
- **Conta Spotify Premium** para o host. É uma exigência do Spotify para apps em modo de desenvolvimento
- As playlists usadas precisam estar na conta do host. A API só devolve as músicas de playlists do próprio usuário

## Configuração

### 1. Criar o app no Spotify

1. Acesse o [Spotify Developer Dashboard](https://developer.spotify.com/dashboard) e crie um app.
2. Em **Redirect URIs**, adicione `http://127.0.0.1:8080/callback`. O Spotify não aceita `localhost`.
3. Copie o **Client ID** e o **Client Secret**.

### 2. Instalar as ferramentas

```powershell
winget install --id GoLang.Go
winget install --id Cloudflare.cloudflared
```

### 3. Rodar

```powershell
$env:SPOTIFY_CLIENT_ID = "seu_client_id"
$env:SPOTIFY_CLIENT_SECRET = "seu_client_secret"

go run .
```

Abra `http://127.0.0.1:8080` e faça login com o Spotify.

### 4. Liberar para os amigos

Em outro terminal:

```powershell
cloudflared tunnel --url http://127.0.0.1:8080
```

O `cloudflared` mostra um endereço `https://….trycloudflare.com`. É esse link que você manda para os amigos. Não precisa abrir portas no roteador. O endereço muda a cada execução.

### Gerar o executável

```powershell
go build -o song-guess.exe .
```

Para Linux: `$env:GOOS = "linux"; go build -o song-guess .`

---

## Estrutura do projeto

```
song-guess/
├── main.go          # servidor HTTP, rotas, embed
├── spotify.go       # OAuth e leitura de playlists
├── deezer.go        # busca de prévias (ISRC) e fallback no iTunes
├── sala.go          # estado da sala e loop da partida
├── ws.go            # conexões WebSocket
├── match.go         # normalização e comparação de palpites
├── web/             # frontend
├── REQUISITOS.md    # regras do jogo
└── PROTOCOLO.md     # mensagens WebSocket
```

---

## Limitações conhecidas

- **Até 5 contas no Spotify por app** em modo de desenvolvimento. Isso não afeta os jogadores, já que só o host faz login.
- **Nem toda música tem prévia** no Deezer ou no iTunes. Essas faixas são puladas automaticamente.
- **O jogo depende do computador do host** estar ligado e conectado durante a partida.
- O token do Spotify expira em 6 meses, e aí o host precisa fazer login de novo.

---

## Sobre o desenvolvimento

O planejamento, a definição de requisitos e o desenvolvimento deste projeto contaram com o auxílio de IA.

## Aviso

Projeto pessoal, sem fins comerciais e sem vínculo com Spotify, Deezer ou Apple. As prévias de áudio são as disponibilizadas publicamente por esses serviços.
