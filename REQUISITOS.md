# Song Guess — Requisitos

## Contexto e stack (definido)

- Linguagem: **Go**, desenvolvimento no **Windows 11**
- Servidor roda local na máquina do host; amigos acessam pela web via **Cloudflare Tunnel**
- **Spotify Web API**: só o host faz login (OAuth); fornece playlists da conta do host e metadados (título, artista, capa, ISRC)
- **Áudio**: prévias de 30s do **Deezer** (busca por ISRC), fallback **iTunes** por nome — a API do Spotify não entrega mais prévias
- Tempo real via **WebSocket** (`github.com/coder/websocket`)
- Endpoints do Spotify pós-fev/2026: `GET /me/playlists`, `GET /playlists/{id}/items` (campo `items[].item`, não `track`)

---

## Requisitos gerais

| ID | Requisito |
|---|---|
| RG-01 | O host cria uma sala, escolhe a playlist e o modo de jogo |
| RG-02 | Amigos entram pelo link com um apelido, sem conta no Spotify |
| RG-03 | O trecho de áudio toca de forma sincronizada para todos os jogadores |
| RG-04 | A resposta (título/banda/capa) só é enviada ao cliente no fim da rodada |
| RG-05 | Faixas sem prévia disponível são puladas automaticamente |
| RG-06 | **"Banda" = artista principal** (primeiro item de `artists`). Artistas convidados são ignorados em todas as regras |
| RG-07 | Uma música não se repete como resposta correta na mesma partida |

## Configuração da sala (host)

| ID | Requisito |
|---|---|
| CF-01 | O host define a **duração do trecho** tocado: **5s, 10s, 15s, 20s ou 30s** |
| CF-02 | O host define o **número de rodadas: de 10 a 50** |
| CF-03 | Se a playlist tiver menos músicas com prévia do que o número de rodadas escolhido, o sistema avisa e limita o máximo ao que está disponível |

> **Por que não tem 40s (CF-01):** as prévias do Deezer e do iTunes têm no máximo 30 segundos. Não existe fonte legítima de trechos mais longos sem o áudio completo, e repetir o trecho em loop não acrescenta informação nova. Opção removida.

## Lobby

| ID | Requisito |
|---|---|
| LB-01 | O lobby mostra a lista de jogadores na sala e quem já está pronto |
| LB-02 | Cada jogador, **inclusive o host**, clica em **"Estou pronto"** antes da partida. Esse clique também libera o áudio (ver RD-00) |
| LB-03 | O botão de iniciar do host **só fica habilitado quando todos clicaram em "Estou pronto"** |
| LB-04 | Se alguém entrar na sala depois de todos estarem prontos, o botão de iniciar **volta a ficar bloqueado** até essa pessoa clicar |

## Durante a rodada

| ID | Requisito |
|---|---|
| RD-00 | Cada rodada **começa tocando a música automaticamente para todos**, ao mesmo tempo |
| RD-01 | Cada rodada tem **2 minutos** para adivinhar, **nos dois modos** |
| RD-02 | Cada jogador controla o **volume só para si** (fica salvo no navegador dele entre rodadas) |
| RD-03 | Cada jogador tem um **botão de replay**, que toca o trecho de novo **só para ele**, a qualquer momento da rodada |
| RD-04 | Existe um **botão de revelar a música**, que mostra quantos já votaram (ex.: "2/3") |
| RD-05 | **Apertar revelar = desistir**: o jogador para de tentar naquela rodada |
| RD-06 | A contagem de "todos" (RD-04) considera **só quem ainda está jogando a rodada**. Quem já terminou (acertou a música, errou no modo 1 ou desistiu) **não precisa votar**. **Quem desconecta sai da contagem** |
| RD-07 | **A rodada termina sozinha** quando todos os jogadores terminaram (acertaram, erraram ou desistiram). Aí revela música e banda para todos |
| RD-08 | Se o timer chegar a zero, a música e a banda são reveladas automaticamente |

> **Restrição técnica de RD-00:** navegadores (principalmente Safari/iPhone) bloqueiam áudio automático até o usuário interagir com a página. O clique em **"Estou pronto"** (LB-02) libera o áudio, e o jogo reutiliza o mesmo player (um único elemento `<audio>` ou `AudioContext`, só trocando a música) em todas as rodadas. Se o navegador bloquear mesmo assim, aparece um botão "Tocar" só para aquele jogador.

> **Nota de RD-06/RD-07:** na prática, a votação de revelar e o fim automático são a mesma regra: a rodada acaba quando ninguém mais está tentando.

## Placar e fim da partida

| ID | Requisito |
|---|---|
| PL-01 | Uma **coluna lateral** mostra todos os jogadores com nome e pontuação total, **ordenada da maior para a menor** |
| PL-02 | A pontuação do placar é **atualizada no fim de cada rodada**, junto com a revelação da música |
| PL-03 | Depois da última rodada, uma tela de pódio revela **em sequência**: primeiro o **3º lugar**, depois o **2º** e por último o **vencedor (1º)** |
| PL-04 | Com menos de 3 jogadores, o pódio mostra só as posições que existem |
| PL-05 | O pódio aparece **só no fim da partida**, nunca entre rodadas. Revelações a cada **3 segundos** |
| PL-06 | **Empate:** vence quem **acertou mais músicas**. Se continuar empatado, os jogadores **dividem a posição** |
| PL-07 | Depois do pódio, o host pode **iniciar outra partida com os mesmos jogadores**: todos voltam ao lobby, a pontuação é zerada e o host pode trocar playlist, modo e configurações. Vale de novo a regra do "Estou pronto" (LB-02 a LB-04) |

### Velocidade (vale para os dois modos)

`P(t)` = pontos pela velocidade no momento do acerto, **linear de 1000 (instantâneo) a 100 (fim dos 2 min)**:

```
P(t) = 1000 - 900 × (t / 120)      // t em segundos desde o início da rodada
```

---

## Modo 1 — Múltipla escolha

| ID | Requisito |
|---|---|
| M1-01 | Cada rodada mostra **4 opções** no formato **"Música — Banda"** |
| M1-02 | Uma opção é a correta; as outras 3 são distratores **sorteados da mesma playlist** |
| M1-03 | As opções são sorteadas aleatoriamente, com no máximo **uma banda repetida uma vez** (2 opções da mesma banda, músicas diferentes). As outras opções são de bandas diferentes. Nenhuma rodada força a repetição |
| M1-04 | **Acertar a música:** `P(t)` pontos |
| M1-05 | **Acertar só a banda** (opção da banda certa, música errada): `40% × P(t)`. Essa opção fica bloqueada e o jogador pode tentar de novo |
| M1-06 | **Banda e depois música:** só completa os pontos até o valor da música no momento do acerto, sem somar os dois. Total = `max(pontos_banda, P(t_música))` |
| M1-07 | **Música primeiro:** encerra a rodada para o jogador, sem pontos extras pela banda |
| M1-08 | **Errar banda e música** encerra a rodada para o jogador. Erro vale 0 pontos, sem descontar |

> **Consequência de M1-03:** o acerto só de banda só é possível quando o sorteio coloca um distrator da mesma banda da correta. Se a banda repetida no sorteio for de dois distratores (nenhum da correta), não há acerto parcial naquela rodada.

**Exemplos:**

| Situação | Pontos |
|---|---|
| Música aos 10s | 925 |
| Só banda aos 10s | 370 |
| Banda aos 10s, música aos 30s | 370 + 405 = **775** (o valor da música aos 30s) |
| Banda aos 10s, depois erra | 370 (rodada encerrada para ele) |
| Erra tudo aos 5s | 0 (rodada encerrada para ele) |

---

## Modo 2 — Digitação

| ID | Requisito |
|---|---|
| M2-01 | O jogador digita **o título** ou **a banda**. Pode também digitar **título + banda** juntos |
| M2-02 | **A cada tentativa, mostra a porcentagem de acerto** |
| M2-03 | **Todos veem a % de cada tentativa dos outros, em tempo real**, conforme acontece (ex.: "Ana: 42%", depois "Ana: 81%"). Quando alguém acerta, aparece **"acertou a banda"** ou **"acertou a música"**. Nunca mostra o texto digitado, para não entregar a resposta |
| M2-04 | **Conta como acerto a partir de 95%** de semelhança |
| M2-05 | **Não exige o nome exato** do título: ignora conteúdo entre parênteses/colchetes, trechos após traço (" - Remastered", " - Ao Vivo"), "feat." / "ft." / "featuring", além de maiúsculas, acentos e pontuação |
| M2-06 | **Título + banda:** vale como acerto da música **só se a banda também estiver certa**. Título certo com banda errada = 0 pontos naquela tentativa |
| M2-07 | O jogador pode tentar várias vezes; errar **não** encerra a rodada. Termina ao acertar a música ou ao desistir (RD-05) |
| M2-08 | **Pontuação:** velocidade pesa mais, número de tentativas pesa em segundo lugar |
| M2-09 | **Acertar só a banda** vale `40%` e pode ser completado depois acertando o título, com a mesma regra do modo 1 (M1-05/M1-06) |

### Pontuação do modo 2

```
pontos_música = P(t) × max(0.5, 1 - 0.05 × (tentativas - 1))
pontos_banda  = 40% × pontos_música calculados no momento do acerto da banda
```

Cada tentativa extra tira 5%, até no máximo 50%. Assim a velocidade continua mandando: acertar rápido com várias tentativas vale mais que acertar devagar na primeira.

| Situação | Pontos |
|---|---|
| Acerta aos 10s na 1ª tentativa | 925 |
| Acerta aos 10s na 4ª tentativa | 925 × 0,85 = **786** |
| Acerta aos 60s na 1ª tentativa | **550** |

### Normalização (M2-05)

**Título de referência** (o que vem do Spotify):

1. Minúsculas e remoção de acentos (`é` → `e`)
2. Remover `(...)` e `[...]`
3. Remover tudo após ` - ` (traço com espaços)
4. Remover `feat.`, `ft.`, `featuring` e o que vier depois
5. Remover pontuação e espaços duplicados

**Palpite do jogador:** só os passos 1 e 5. Os passos 2–4 **não** são aplicados no palpite, senão "Paranoid - Banda Errada" viraria "paranoid" e passaria, contrariando M2-06.

| Título no Spotify | Normalizado |
|---|---|
| Bohemian Rhapsody - Remastered 2011 | bohemian rhapsody |
| Despacito (feat. Justin Bieber) - Remix | despacito |
| Ela É Demais | ela e demais |

### Comparação (M2-01, M2-04, M2-06)

O palpite é comparado com estas referências, e vale a de maior %:

| Referência | Se ≥ 95% |
|---|---|
| título | acerto da música |
| título completo, só minúsculas/sem acento (para "(I Can't Get No) Satisfaction") | acerto da música |
| título + banda / banda + título | acerto da música |
| banda | acerto da banda |

Título certo + banda errada não chega a 95% em nenhuma referência, então dá 0 automaticamente (M2-06).

**Cálculo da %:** `1 - distância_levenshtein / tamanho_maior`.

---

## A decidir

Nada em aberto.
