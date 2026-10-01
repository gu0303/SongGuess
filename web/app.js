/* ==========================================================
   Song Guess — Client Application
   Web Audio API, Game State Machine & High-Precision Timing
   (Sem login: carrega qualquer link do Spotify ou Deezer)
   ========================================================== */

(function () {
  'use strict';

  // --- AUDIO ENGINE (Web Audio API) ---
  class SoundEngine {
    constructor() {
      this.ctx = null;
      this.gainNode = null;
      this.currentBuffer = null;
      this.currentSource = null;
      this.clipSeconds = 10;
      this.isPlaying = false;
      this.isPaused = false;
      this.playbackOffset = 0;
      this.startedAt = 0;
      this.onStart = null;
      this.onEnd = null;
      this.onPause = null;
      this.volume = parseFloat(localStorage.getItem('song_guess_volume') || '0.8');
    }

    init() {
      if (!this.ctx) {
        const AudioCtx = window.AudioContext || window.webkitAudioContext;
        this.ctx = new AudioCtx();
        this.gainNode = this.ctx.createGain();
        this.gainNode.gain.setValueAtTime(this.volume, this.ctx.currentTime);
        this.gainNode.connect(this.ctx.destination);
      }
      if (this.ctx.state === 'suspended') {
        this.ctx.resume();
      }
    }

    setVolume(vol) {
      this.volume = Math.max(0, Math.min(1, vol));
      localStorage.setItem('song_guess_volume', this.volume.toString());
      if (this.gainNode && this.ctx) {
        this.gainNode.gain.setValueAtTime(this.volume, this.ctx.currentTime);
      }
    }

    async loadAndPlay(audioUrl, clipSeconds, onStart, onEnd, onPause) {
      this.init();
      this.stop();
      this.clipSeconds = clipSeconds;
      this.onStart = onStart;
      this.onEnd = onEnd;
      this.onPause = onPause;

      try {
        const resp = await fetch(audioUrl);
        const arrayBuf = await resp.arrayBuffer();
        this.currentBuffer = await this.ctx.decodeAudioData(arrayBuf);
        this.playbackOffset = 0;
        this.playClip(onStart, onEnd, onPause, 0);
      } catch (err) {
        console.error('Audio load/decode error:', err);
        if (onEnd) onEnd();
      }
    }

    playClip(onStart, onEnd, onPause, offset = 0) {
      if (!this.currentBuffer || !this.ctx) return;
      this.stopSourceOnly();

      if (onStart) this.onStart = onStart;
      if (onEnd) this.onEnd = onEnd;
      if (onPause) this.onPause = onPause;

      this.playbackOffset = Math.max(0, offset);
      const totalDuration = this.clipSeconds || 10;
      const remaining = Math.max(0.1, totalDuration - this.playbackOffset);

      this.currentSource = this.ctx.createBufferSource();
      this.currentSource.buffer = this.currentBuffer;
      this.currentSource.connect(this.gainNode);

      this.isPlaying = true;
      this.isPaused = false;
      this.startedAt = this.ctx.currentTime;

      if (this.onStart) this.onStart();

      // AudioBufferSourceNode.start(when, offset, duration)
      this.currentSource.start(0, this.playbackOffset, remaining);

      this.currentSource.onended = () => {
        // Only trigger onEnd if it ended naturally while playing (not via pause/stop)
        if (this.isPlaying) {
          this.isPlaying = false;
          this.isPaused = false;
          this.playbackOffset = 0;
          this.currentSource = null;
          if (this.onEnd) this.onEnd();
        }
      };
    }

    pause() {
      if (!this.isPlaying || !this.ctx || !this.currentSource) return;

      const elapsed = this.ctx.currentTime - this.startedAt;
      this.playbackOffset = Math.min(this.clipSeconds, this.playbackOffset + elapsed);

      this.isPlaying = false;
      this.isPaused = true;

      this.currentSource.onended = null;
      try {
        this.currentSource.stop();
        this.currentSource.disconnect();
      } catch (e) {}
      this.currentSource = null;

      if (this.onPause) this.onPause();
    }

    resume() {
      if (!this.currentBuffer) return;
      if (this.playbackOffset >= this.clipSeconds) {
        this.playbackOffset = 0;
      }
      this.playClip(this.onStart, this.onEnd, this.onPause, this.playbackOffset);
    }

    toggle() {
      if (this.isPlaying) {
        this.pause();
      } else {
        this.resume();
      }
    }

    stopSourceOnly() {
      if (this.currentSource) {
        this.currentSource.onended = null;
        try {
          this.currentSource.stop();
          this.currentSource.disconnect();
        } catch (e) {}
        this.currentSource = null;
      }
      this.isPlaying = false;
    }

    stop() {
      this.stopSourceOnly();
      this.isPlaying = false;
      this.isPaused = false;
      this.playbackOffset = 0;
    }
  }

  const audio = new SoundEngine();

  // --- GAME STATE ---
  const state = {
    mode: 1, // 1: Múltipla Escolha, 2: Digitação
    clipSeconds: 10,
    rounds: 10,
    playlistId: null,
    playlistName: '',
    lastLoadedUrl: '',
    playerName: 'Jogador',
    currentRound: null,
    totalScore: 0,
    correctSongs: 0,
    roundTimerInterval: null,
    speedTickerInterval: null,
    startsAtMs: 0,
    endsAtMs: 0,
  };

  // --- DOM ELEMENTS ---
  const el = {
    // Header
    brandLogo: document.getElementById('brand-logo'),
    ticker: document.getElementById('game-ticker'),
    tickerRound: document.getElementById('ticker-round'),
    tickerScore: document.getElementById('ticker-score'),
    tickerHits: document.getElementById('ticker-hits'),
    volumeSlider: document.getElementById('volume-slider'),
    volumeVal: document.getElementById('volume-val'),
    volumeToggleBtn: document.getElementById('volume-toggle-btn'),
    volumeIcon: document.getElementById('volume-icon'),

    // Views
    viewLobby: document.getElementById('view-lobby'),
    viewGame: document.getElementById('view-game'),
    viewPodium: document.getElementById('view-podium'),
    roundModal: document.getElementById('round-reveal-modal'),

    // Lobby
    playerNameInput: document.getElementById('player-name-input'),
    modeBtn1: document.getElementById('mode-btn-1'),
    modeBtn2: document.getElementById('mode-btn-2'),
    clipSelector: document.getElementById('clip-selector'),
    roundsSelect: document.getElementById('rounds-select'),
    playlistUrlInput: document.getElementById('playlist-url-input'),
    btnLoadPlaylist: document.getElementById('btn-load-playlist'),
    urlFeedback: document.getElementById('url-feedback'),
    playlistEmptyState: document.getElementById('playlist-empty-state'),
    loadedPlaylistPreview: document.getElementById('loaded-playlist-preview'),
    previewCover: document.getElementById('preview-cover'),
    previewName: document.getElementById('preview-name'),
    previewDesc: document.getElementById('preview-desc'),
    previewCount: document.getElementById('preview-count'),
    previewSource: document.getElementById('preview-source'),
    selectedPlaylistName: document.getElementById('selected-playlist-name'),
    btnStartGame: document.getElementById('btn-start-game'),

    // Game Board
    gameRoundNum: document.getElementById('game-round-num'),
    gameTotalRounds: document.getElementById('game-total-rounds'),
    gameClipBadge: document.getElementById('game-clip-badge'),
    timerText: document.getElementById('timer-text'),
    timerBarFill: document.getElementById('timer-bar-fill'),
    gameScoreVal: document.getElementById('game-score-val'),
    vinylDisc: document.getElementById('vinyl-disc'),
    vinylCenter: document.getElementById('vinyl-center'),
    vinylOverlayBadge: document.getElementById('vinyl-overlay-badge'),
    vinylOverlayIcon: document.getElementById('vinyl-overlay-icon'),
    soundwave: document.getElementById('soundwave'),
    audioStateBadge: document.getElementById('audio-state-badge'),
    audioStateText: document.getElementById('audio-state-text'),
    speedVal: document.getElementById('speed-val'),
    btnReplay: document.getElementById('btn-replay'),
    btnReveal: document.getElementById('btn-reveal'),

    // Panels
    panelMode1: document.getElementById('panel-mode-1'),
    optionsGrid: document.getElementById('options-grid'),
    panelMode2: document.getElementById('panel-mode-2'),
    similarityPct: document.getElementById('similarity-pct'),
    similarityBar: document.getElementById('similarity-bar'),
    meterStatusMsg: document.getElementById('meter-status-msg'),
    guessForm: document.getElementById('guess-form'),
    guessInput: document.getElementById('guess-input'),
    btnSubmitGuess: document.getElementById('btn-submit-guess'),
    attemptsList: document.getElementById('attempts-list'),

    // Reveal Modal
    revealIcon: document.getElementById('reveal-icon'),
    revealTitle: document.getElementById('reveal-title'),
    revealCover: document.getElementById('reveal-cover'),
    revealTrackName: document.getElementById('reveal-track-name'),
    revealArtistName: document.getElementById('reveal-artist-name'),
    revealAlbumName: document.getElementById('reveal-album-name'),
    revealPtsVal: document.getElementById('reveal-pts-val'),
    revealTotalVal: document.getElementById('reveal-total-val'),
    btnNextRound: document.getElementById('btn-next-round'),

    // Podium
    podiumRank: document.getElementById('podium-rank'),
    podiumPlayerCongrats: document.getElementById('podium-player-congrats'),
    podiumTotalScore: document.getElementById('podium-total-score'),
    podiumCorrectCount: document.getElementById('podium-correct-count'),
    podiumAccuracyPct: document.getElementById('podium-accuracy-pct'),
    podiumModeName: document.getElementById('podium-mode-name'),
    reviewTracksList: document.getElementById('review-tracks-list'),
    btnPlayAgain: document.getElementById('btn-play-again'),

    // Confirm modal
    confirmOverlay: document.getElementById('confirm-overlay'),
    confirmBtnOk: document.getElementById('confirm-btn-ok'),
    confirmBtnCancel: document.getElementById('confirm-btn-cancel'),
  };

  // --- INITIALIZATION ---
  async function init() {
    setupEventListeners();
    setupVolumeUI();
  }

  function setupVolumeUI() {
    const volPct = Math.round(audio.volume * 100);
    el.volumeSlider.value = volPct;
    el.volumeVal.textContent = `${volPct}%`;
    updateVolumeIcon(volPct);
  }

  function updateVolumeIcon(vol) {
    if (vol === 0) {
      el.volumeIcon.textContent = '🔇';
    } else if (vol < 50) {
      el.volumeIcon.textContent = '🔉';
    } else {
      el.volumeIcon.textContent = '🔊';
    }
  }

  // --- PLAYLIST LOADER ---
  async function loadPlaylist(inputUrlOrId) {
    const val = (inputUrlOrId || el.playlistUrlInput.value || '').trim();
    if (!val) {
      showFeedback('Por favor, cole o link de uma playlist do Spotify ou Deezer.', 'error');
      el.playlistUrlInput.focus();
      return false;
    }

    el.btnLoadPlaylist.disabled = true;
    el.btnLoadPlaylist.textContent = 'Carregando...';
    showFeedback('Buscando músicas da playlist no Spotify/Deezer...', 'info');

    try {
      const res = await fetch('/api/playlist/load', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ url: val }),
      });

      const data = await res.json();
      if (!res.ok) {
        showFeedback(data.error || 'Não foi possível carregar a playlist. Verifique se o link está correto e público.', 'error');
        state.playlistId = null;
        state.playlistName = '';
        if (el.loadedPlaylistPreview) el.loadedPlaylistPreview.style.display = 'none';
        if (el.playlistEmptyState) el.playlistEmptyState.style.display = 'flex';
        el.selectedPlaylistName.textContent = 'Nenhuma (cole o link acima)';
        el.selectedPlaylistName.classList.add('unselected');
        return false;
      }

      // Success
      state.playlistId = data.id;
      state.playlistName = data.name;
      state.lastLoadedUrl = val;

      el.previewName.textContent = data.name;
      el.previewDesc.textContent = data.description || '';
      el.previewCount.textContent = `${data.totalTracks} músicas disponíveis para jogar`;
      el.selectedPlaylistName.textContent = data.name;
      el.selectedPlaylistName.classList.remove('unselected');

      if (data.coverUrl) {
        el.previewCover.src = data.coverUrl;
      }

      el.previewSource.textContent = data.source === 'spotify' ? '🟢 Spotify' : '⚡ Deezer';

      if (el.playlistEmptyState) el.playlistEmptyState.style.display = 'none';
      if (el.loadedPlaylistPreview) el.loadedPlaylistPreview.style.display = 'flex';

      showFeedback(`✓ Playlist pronta: "${data.name}" (${data.totalTracks} músicas carregadas)`, 'success');
      return true;
    } catch (e) {
      console.error(e);
      showFeedback('Erro de conexão ao carregar a playlist.', 'error');
      return false;
    } finally {
      el.btnLoadPlaylist.disabled = false;
      el.btnLoadPlaylist.textContent = 'Carregar';
    }
  }

  function showFeedback(msg, type) {
    el.urlFeedback.style.display = 'block';
    el.urlFeedback.className = `url-feedback ${type || 'info'}`;
    el.urlFeedback.textContent = msg;
  }

  function hideFeedback() {
    el.urlFeedback.style.display = 'none';
  }

  // --- EVENT LISTENERS ---
  function setupEventListeners() {
    // Mode toggles
    el.modeBtn1.onclick = () => setMode(1);
    el.modeBtn2.onclick = () => setMode(2);

    // Clip Duration pills
    el.clipSelector.querySelectorAll('.clip-pill').forEach(btn => {
      btn.onclick = () => {
        el.clipSelector.querySelectorAll('.clip-pill').forEach(b => b.classList.remove('active'));
        btn.classList.add('active');
        state.clipSeconds = parseInt(btn.dataset.clip, 10);
      };
    });

    // Rounds Select
    el.roundsSelect.onchange = (e) => {
      state.rounds = parseInt(e.target.value, 10);
    };

    // Load Playlist button and enter key
    el.btnLoadPlaylist.onclick = () => loadPlaylist();
    el.playlistUrlInput.onkeydown = (e) => {
      if (e.key === 'Enter') {
        e.preventDefault();
        loadPlaylist();
      }
    };

    // Auto-load on paste
    el.playlistUrlInput.addEventListener('paste', () => {
      setTimeout(() => {
        const val = el.playlistUrlInput.value.trim();
        if (val) {
          loadPlaylist(val);
        }
      }, 50);
    });

    // Auto-load on blur if changed
    el.playlistUrlInput.addEventListener('blur', () => {
      const val = el.playlistUrlInput.value.trim();
      if (val && val !== state.lastLoadedUrl) {
        loadPlaylist(val);
      }
    });

    // Auto-load on input with debounce
    let urlDebounce = null;
    el.playlistUrlInput.addEventListener('input', () => {
      clearTimeout(urlDebounce);
      const val = el.playlistUrlInput.value.trim();
      if (val.length > 20 && (val.includes('spotify.com') || val.includes('deezer.com') || val.includes('.link'))) {
        urlDebounce = setTimeout(() => {
          if (val !== state.lastLoadedUrl) {
            loadPlaylist(val);
          }
        }, 600);
      }
    });

    // Volume
    el.volumeSlider.oninput = (e) => {
      const val = parseInt(e.target.value, 10);
      audio.setVolume(val / 100);
      el.volumeVal.textContent = `${val}%`;
      updateVolumeIcon(val);
    };

    el.volumeToggleBtn.onclick = () => {
      if (audio.volume > 0) {
        audio.setVolume(0);
        el.volumeSlider.value = 0;
        el.volumeVal.textContent = '0%';
        updateVolumeIcon(0);
      } else {
        audio.setVolume(0.8);
        el.volumeSlider.value = 80;
        el.volumeVal.textContent = '80%';
        updateVolumeIcon(80);
      }
    };

    // Start Game
    el.btnStartGame.onclick = startGame;

    // In-game controls: Replay & Vinyl click to pause/unpause
    el.btnReplay.onclick = () => {
      audio.playClip(onAudioPlayStart, onAudioPlayEnd, onAudioPause, 0);
    };

    const toggleVinylPlayback = (e) => {
      if (e) e.preventDefault();
      if (state.currentRound && audio.currentBuffer) {
        audio.toggle();
      }
    };

    el.vinylDisc.onclick = toggleVinylPlayback;
    el.vinylDisc.onkeydown = (e) => {
      if (e.code === 'Space' || e.code === 'Enter') {
        toggleVinylPlayback(e);
      }
    };

    el.btnReveal.onclick = handleReveal;

    // Mode 2 Guess (Prevent default form submission / page reload)
    if (el.guessForm) {
      el.guessForm.onsubmit = (e) => {
        if (e) {
          e.preventDefault();
          e.stopPropagation();
        }
        submitGuess(e);
        return false;
      };
    }

    if (el.btnSubmitGuess) {
      el.btnSubmitGuess.onclick = (e) => {
        if (e) {
          e.preventDefault();
          e.stopPropagation();
        }
        submitGuess(e);
      };
    }

    if (el.guessInput) {
      el.guessInput.onkeydown = (e) => {
        if (e.key === 'Enter') {
          e.preventDefault();
          e.stopPropagation();
          submitGuess(e);
        }
      };
    }

    // Next Round modal
    el.btnNextRound.onclick = loadNextRound;

    // Play again
    el.btnPlayAgain.onclick = returnToLobby;
    el.brandLogo.onclick = handleLogoClick;

    // Confirm modal buttons
    el.confirmBtnOk.onclick = () => {
      hideConfirmModal();
      returnToLobby();
    };
    el.confirmBtnCancel.onclick = hideConfirmModal;
    el.confirmOverlay.addEventListener('click', (e) => {
      if (e.target === el.confirmOverlay) hideConfirmModal();
    });
    document.addEventListener('keydown', (e) => {
      if (e.key === 'Escape' && el.confirmOverlay.style.display !== 'none') {
        hideConfirmModal();
      }
    });
  }

  function setMode(modeNum) {
    state.mode = modeNum;
    if (modeNum === 1) {
      el.modeBtn1.classList.add('active');
      el.modeBtn2.classList.remove('active');
    } else {
      el.modeBtn2.classList.add('active');
      el.modeBtn1.classList.remove('active');
    }
  }

  // --- START GAME WORKFLOW ---
  async function startGame() {
    audio.init(); // unlock Web Audio on user gesture (RD-00)
    state.playerName = el.playerNameInput.value.trim() || 'Jogador';

    const inputUrl = el.playlistUrlInput.value.trim();

    // If no playlist loaded yet, try to load from the input field
    if (!state.playlistId) {
      if (!inputUrl) {
        showFeedback('Por favor, cole o link de uma playlist do Spotify ou Deezer acima antes de iniciar!', 'error');
        el.playlistUrlInput.focus();
        return;
      }
      el.btnStartGame.disabled = true;
      el.btnStartGame.innerHTML = '<span class="spinner-small"></span> Carregando playlist...';
      const success = await loadPlaylist(inputUrl);
      if (!success || !state.playlistId) {
        el.btnStartGame.disabled = false;
        el.btnStartGame.innerHTML = '<span class="btn-icon">▶</span><span>Estou Pronto / Iniciar Jogo</span>';
        return;
      }
    } else if (inputUrl && inputUrl !== state.lastLoadedUrl) {
      // User changed URL in the input field without clicking load
      el.btnStartGame.disabled = true;
      el.btnStartGame.innerHTML = '<span class="spinner-small"></span> Carregando playlist...';
      const success = await loadPlaylist(inputUrl);
      if (!success || !state.playlistId) {
        el.btnStartGame.disabled = false;
        el.btnStartGame.innerHTML = '<span class="btn-icon">▶</span><span>Estou Pronto / Iniciar Jogo</span>';
        return;
      }
    }

    el.btnStartGame.disabled = true;
    el.btnStartGame.innerHTML = '<span class="spinner-small"></span> Preparando partida...';

    try {
      const payload = {
        mode: state.mode,
        clipSeconds: state.clipSeconds,
        rounds: state.rounds,
        playlistId: state.playlistId,
        playlistName: state.playlistName,
        playerName: state.playerName,
      };

      const res = await fetch('/api/game/start', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });

      if (!res.ok) {
        const err = await res.json();
        alert(err.error || 'Erro ao iniciar partida');
        el.btnStartGame.disabled = false;
        el.btnStartGame.innerHTML = '<span class="btn-icon">▶</span><span>Estou Pronto / Iniciar Jogo</span>';
        return;
      }

      // Switch to Game Board view
      showView('game');
      el.ticker.style.display = 'flex';
      updateHeaderTicker(1, state.rounds, 0, 0);

      // Start first round
      await loadNextRound();
    } catch (e) {
      console.error(e);
      alert('Erro de conexão ao iniciar jogo.');
    } finally {
      el.btnStartGame.disabled = false;
      el.btnStartGame.innerHTML = '<span class="btn-icon">▶</span><span>Estou Pronto / Iniciar Jogo</span>';
    }
  }

  // --- ROUND LIFECYCLE ---
  async function loadNextRound() {
    el.roundModal.style.display = 'none';
    clearInterval(state.roundTimerInterval);
    clearInterval(state.speedTickerInterval);
    audio.stop();

    try {
      const res = await fetch('/api/game/round/next', { method: 'POST' });
      if (!res.ok) {
        await showPodium();
        return;
      }

      const rState = await res.json();
      state.currentRound = rState;
      state.totalScore = rState.totalScore;
      state.correctSongs = rState.correctSongs;
      state.startsAtMs = rState.startsAtUnixMs;
      state.endsAtMs = rState.endsAtUnixMs;

      setupRoundUI(rState);

      // Play clip automatically
      audio.loadAndPlay(rState.audioUrl, rState.clipSeconds, onAudioPlayStart, onAudioPlayEnd, onAudioPause);

      // Start 2-minute timer
      startRoundTimer();
      startSpeedTicker();
    } catch (e) {
      console.error('Error loading next round:', e);
      alert('Falha ao carregar a próxima rodada.');
    }
  }

  function setupRoundUI(rState) {
    el.gameRoundNum.textContent = rState.roundNumber;
    el.gameTotalRounds.textContent = rState.totalRounds;
    el.gameClipBadge.textContent = `Trecho: ${rState.clipSeconds}s`;
    el.gameScoreVal.textContent = rState.totalScore;
    updateHeaderTicker(rState.roundNumber, rState.totalRounds, rState.totalScore, rState.correctSongs);

    // Reset spinning vinyl disc and center cover
    if (el.vinylDisc) {
      el.vinylDisc.classList.remove('spinning', 'paused', 'ended');
      if (el.vinylOverlayIcon) el.vinylOverlayIcon.textContent = '⏸';
    }
    if (el.vinylCenter) {
      el.vinylCenter.style.backgroundImage = 'none';
      el.vinylCenter.textContent = '🎵';
      el.vinylCenter.classList.remove('has-cover');
    }

    // Setup mode-specific UI
    if (state.mode === 1) {
      el.panelMode1.style.display = 'block';
      el.panelMode2.style.display = 'none';
      renderMode1Options(rState.options);
    } else {
      el.panelMode1.style.display = 'none';
      el.panelMode2.style.display = 'block';
      resetMode2UI();
    }
  }

  function renderMode1Options(options) {
    el.optionsGrid.innerHTML = '';
    options.forEach((opt, idx) => {
      const btn = document.createElement('button');
      btn.type = 'button';
      btn.className = 'option-btn';
      btn.id = `opt-${opt.id}`;
      btn.disabled = opt.disabled;

      const parts = opt.label.split(' — ');
      const songTitle = parts[0] || opt.label;
      const artist = parts[1] || '';

      btn.innerHTML = `
        <span class="opt-badge">Opção ${idx + 1}</span>
        <div class="opt-song">${escapeHtml(songTitle)}</div>
        <div class="opt-artist">${escapeHtml(artist)}</div>
      `;

      btn.onclick = () => chooseOption(opt.id, btn);
      el.optionsGrid.appendChild(btn);
    });
  }

  async function chooseOption(optionId, btnElement) {
    try {
      const res = await fetch('/api/game/round/choose', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ optionId }),
      });

      if (!res.ok) return;

      const rState = await res.json();
      state.currentRound = rState;
      state.totalScore = rState.totalScore;
      state.correctSongs = rState.correctSongs;
      el.gameScoreVal.textContent = rState.totalScore;
      updateHeaderTicker(rState.roundNumber, rState.totalRounds, rState.totalScore, rState.correctSongs);

      if (rState.finished) {
        clearInterval(state.roundTimerInterval);
        clearInterval(state.speedTickerInterval);
        audio.stop();

        if (rState.answer && btnElement.innerText.includes(rState.answer.title)) {
          btnElement.classList.add('correct-song');
        } else {
          btnElement.classList.add('wrong');
        }

        setTimeout(() => showRevealModal(rState), 600);
      } else if (rState.hasHitBand) {
        btnElement.classList.add('correct-band');
        btnElement.disabled = true;
        const badge = btnElement.querySelector('.opt-badge');
        if (badge) badge.textContent = 'Banda Certa! (+40%)';
      }
    } catch (e) {
      console.error(e);
    }
  }

  function resetMode2UI() {
    el.guessInput.value = '';
    el.guessInput.disabled = false;
    el.guessInput.focus();
    el.similarityPct.textContent = '0%';
    el.similarityBar.style.width = '0%';
    el.meterStatusMsg.className = 'meter-status-msg';
    el.meterStatusMsg.textContent = 'Comece a digitar para testar seus palpites!';
    el.attemptsList.innerHTML = '<div class="empty-history">Nenhum palpite enviado ainda nesta rodada.</div>';
  }

  async function submitGuess(e) {
    if (e) {
      if (e.preventDefault) e.preventDefault();
      if (e.stopPropagation) e.stopPropagation();
    }
    const rawText = el.guessInput.value.trim();
    if (!rawText) return;

    try {
      const res = await fetch('/api/game/round/guess', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ guess: rawText }),
      });

      if (!res.ok) return;

      const rState = await res.json();
      state.currentRound = rState;
      state.totalScore = rState.totalScore;
      state.correctSongs = rState.correctSongs;
      el.gameScoreVal.textContent = rState.totalScore;
      updateHeaderTicker(rState.roundNumber, rState.totalRounds, rState.totalScore, rState.correctSongs);

      const history = rState.history || [];
      const last = history[history.length - 1];

      if (last) {
        el.similarityPct.textContent = `${last.percent}%`;
        el.similarityBar.style.width = `${last.percent}%`;
        renderAttemptsList(history);

        if (rState.finished) {
          clearInterval(state.roundTimerInterval);
          clearInterval(state.speedTickerInterval);
          audio.stop();
          el.meterStatusMsg.className = 'meter-status-msg hit-song';
          el.meterStatusMsg.textContent = '🎉 Acertou a música!';
          el.guessInput.disabled = true;
          setTimeout(() => showRevealModal(rState), 600);
        } else if (rState.hasHitBand) {
          el.meterStatusMsg.className = 'meter-status-msg hit-band';
          el.meterStatusMsg.textContent = '🎸 Acertou a banda (+40%)! Agora tente o nome da música!';
        } else {
          el.meterStatusMsg.className = 'meter-status-msg';
          el.meterStatusMsg.textContent = `Tentativa ${last.attemptNumber}: ${last.percent}% de semelhança.`;
        }
      }

      el.guessInput.value = '';
      el.guessInput.focus();
    } catch (e) {
      console.error(e);
    }
  }

  function renderAttemptsList(history) {
    el.attemptsList.innerHTML = '';
    const reversed = [...history].reverse();
    reversed.forEach(item => {
      const row = document.createElement('div');
      row.className = 'attempt-row';

      let badgeClass = 'low';
      if (item.percent >= 90) badgeClass = 'high';
      else if (item.percent >= 60) badgeClass = 'mid';

      row.innerHTML = `
        <span class="attempt-guess-text">#${item.attemptNumber}: "${escapeHtml(item.normalized)}"</span>
        <span class="attempt-pct-badge ${badgeClass}">${item.percent}%</span>
      `;
      el.attemptsList.appendChild(row);
    });
  }

  // --- REVEAL / TIMEOUT ---
  async function handleReveal() {
    if (!confirm('Deseja desistir desta rodada e revelar a resposta?')) return;

    try {
      const res = await fetch('/api/game/round/reveal', { method: 'POST' });
      if (!res.ok) return;

      const rState = await res.json();
      state.currentRound = rState;
      clearInterval(state.roundTimerInterval);
      clearInterval(state.speedTickerInterval);
      audio.stop();
      showRevealModal(rState);
    } catch (e) {
      console.error(e);
    }
  }

  async function handleTimeout() {
    try {
      const res = await fetch('/api/game/round/timeout', { method: 'POST' });
      if (!res.ok) return;

      const rState = await res.json();
      state.currentRound = rState;
      clearInterval(state.roundTimerInterval);
      clearInterval(state.speedTickerInterval);
      audio.stop();
      showRevealModal(rState);
    } catch (e) {
      console.error(e);
    }
  }

  // --- REVEAL MODAL ---
  function showRevealModal(rState) {
    const ans = rState.answer;
    if (!ans) return;

    const cover = ans.coverUrl || 'https://images.unsplash.com/photo-1511671782779-c97d3d27a1d4?w=500&auto=format&fit=crop&q=80';
    el.revealCover.src = cover;
    el.revealTrackName.textContent = ans.title;
    el.revealArtistName.textContent = ans.artist;
    el.revealAlbumName.textContent = ans.album;

    // Display individual album cover on vinyl disc center as well
    if (el.vinylCenter && cover) {
      el.vinylCenter.style.backgroundImage = `url("${cover}")`;
      el.vinylCenter.textContent = '';
      el.vinylCenter.classList.add('has-cover');
    }
    if (el.vinylDisc) {
      el.vinylDisc.classList.remove('paused');
    }

    if (rState.roundPoints > 0) {
      el.revealIcon.textContent = '🎉';
      el.revealTitle.className = 'result-title success';
      el.revealTitle.textContent = rState.hasHitBand && rState.roundPoints === rState.bandPoints ? 'Acertou a Banda!' : 'Você Acertou!';
      el.revealPtsVal.textContent = `+${rState.roundPoints} pts`;
    } else {
      el.revealIcon.textContent = '⏱️';
      el.revealTitle.className = 'result-title missed';
      el.revealTitle.textContent = 'Fim da Rodada!';
      el.revealPtsVal.textContent = '0 pts';
    }

    el.revealTotalVal.textContent = `${rState.totalScore} pts`;
    el.roundModal.style.display = 'flex';
  }

  // --- PODIUM / FINAL SUMMARY ---
  async function showPodium() {
    try {
      const res = await fetch('/api/game/summary');
      const sum = await res.json();

      el.podiumRank.textContent = sum.titleRank;
      el.podiumPlayerCongrats.textContent = `Parabéns pela partida, ${sum.playerName}!`;
      el.podiumTotalScore.textContent = sum.totalScore.toLocaleString('pt-BR');
      el.podiumCorrectCount.textContent = `${sum.correctSongs}/${sum.totalRounds}`;
      el.podiumAccuracyPct.textContent = `${sum.accuracyPercent}% de precisão`;
      el.podiumModeName.textContent = sum.mode === 1 ? 'Múltipla Escolha' : 'Digitação';

      el.reviewTracksList.innerHTML = '';
      (sum.roundsSummary || []).forEach(tr => {
        const item = document.createElement('div');
        item.className = 'review-track-item';
        const cover = tr.coverUrl || 'https://images.unsplash.com/photo-1511671782779-c97d3d27a1d4?w=500&auto=format&fit=crop&q=80';
        item.innerHTML = `
          <img class="review-track-thumb" src="${cover}" alt="${tr.title}">
          <div class="review-track-info">
            <div class="review-track-title">${escapeHtml(tr.title)}</div>
            <div class="review-track-artist">${escapeHtml(tr.artist)}</div>
          </div>
        `;
        el.reviewTracksList.appendChild(item);
      });

      showView('podium');
    } catch (e) {
      console.error('Error fetching summary:', e);
      returnToLobby();
    }
  }

  function isGameActive() {
    return el.viewGame && el.viewGame.style.display !== 'none';
  }

  function handleLogoClick() {
    if (isGameActive()) {
      showConfirmModal();
    } else {
      returnToLobby();
    }
  }

  function showConfirmModal() {
    el.confirmOverlay.style.display = 'flex';
    // Re-trigger animation by forcing reflow
    el.confirmOverlay.offsetHeight;
    el.confirmBtnCancel.focus();
  }

  function hideConfirmModal() {
    el.confirmOverlay.style.display = 'none';
  }

  function returnToLobby() {
    clearInterval(state.roundTimerInterval);
    clearInterval(state.speedTickerInterval);
    audio.stop();
    if (el.vinylDisc) {
      el.vinylDisc.classList.remove('spinning', 'paused', 'ended');
    }
    if (el.vinylCenter) {
      el.vinylCenter.style.backgroundImage = 'none';
      el.vinylCenter.textContent = '🎵';
      el.vinylCenter.classList.remove('has-cover');
    }
    if (el.audioStateBadge) {
      el.audioStateBadge.classList.remove('badge-paused');
    }
    el.roundModal.style.display = 'none';
    el.ticker.style.display = 'none';
    showView('lobby');
  }

  // --- TIMERS & TICKERS ---
  function startRoundTimer() {
    const totalDuration = 120000;
    const update = () => {
      const now = Date.now();
      const remaining = Math.max(0, state.endsAtMs - now);

      const mins = Math.floor(remaining / 60000);
      const secs = Math.floor((remaining % 60000) / 1000);
      el.timerText.textContent = `${String(mins).padStart(2, '0')}:${String(secs).padStart(2, '0')}`;

      const pctRemaining = (remaining / totalDuration) * 100;
      el.timerBarFill.style.width = `${pctRemaining}%`;

      if (pctRemaining < 25) {
        el.timerBarFill.className = 'timer-bar-fill danger';
      } else if (pctRemaining < 50) {
        el.timerBarFill.className = 'timer-bar-fill warning';
      } else {
        el.timerBarFill.className = 'timer-bar-fill';
      }

      if (remaining <= 0) {
        clearInterval(state.roundTimerInterval);
        handleTimeout();
      }
    };

    update();
    state.roundTimerInterval = setInterval(update, 250);
  }

  function startSpeedTicker() {
    const update = () => {
      const now = Date.now();
      const elapsedSec = Math.max(0, (now - state.startsAtMs) / 1000);
      const pts = Math.max(100, Math.round(1000 - (900 * (elapsedSec / 120))));
      el.speedVal.textContent = pts;
    };
    update();
    state.speedTickerInterval = setInterval(update, 250);
  }

  // --- AUDIO UI SYNC ---
  function onAudioPlayStart() {
    el.vinylDisc.classList.remove('paused', 'ended');
    el.vinylDisc.classList.add('spinning');
    if (el.vinylOverlayIcon) el.vinylOverlayIcon.textContent = '⏸';
    el.soundwave.classList.add('active');
    el.audioStateBadge.style.display = 'inline-flex';
    el.audioStateBadge.classList.remove('badge-paused');
    el.audioStateText.textContent = `Tocando trecho (${state.clipSeconds}s)... (clique no vinil para pausar)`;
  }

  function onAudioPause() {
    el.vinylDisc.classList.remove('ended');
    el.vinylDisc.classList.add('spinning', 'paused');
    if (el.vinylOverlayIcon) el.vinylOverlayIcon.textContent = '▶';
    el.soundwave.classList.remove('active');
    el.audioStateBadge.style.display = 'inline-flex';
    el.audioStateBadge.classList.add('badge-paused');
    el.audioStateText.textContent = 'Pausado (clique no vinil para continuar)';
  }

  function onAudioPlayEnd() {
    el.vinylDisc.classList.remove('spinning', 'paused');
    el.vinylDisc.classList.add('ended');
    if (el.vinylOverlayIcon) el.vinylOverlayIcon.textContent = '▶';
    el.soundwave.classList.remove('active');
    el.audioStateBadge.classList.remove('badge-paused');
    el.audioStateText.textContent = 'Trecho concluído. Clique no vinil para ouvir de novo!';
  }

  // --- HELPERS ---
  function showView(viewName) {
    el.viewLobby.style.display = viewName === 'lobby' ? 'block' : 'none';
    el.viewGame.style.display = viewName === 'game' ? 'block' : 'none';
    el.viewPodium.style.display = viewName === 'podium' ? 'block' : 'none';
    window.scrollTo({ top: 0, behavior: 'smooth' });
  }

  function updateHeaderTicker(round, totalRounds, score, hits) {
    el.tickerRound.textContent = `${round}/${totalRounds}`;
    el.tickerScore.textContent = `${score.toLocaleString('pt-BR')} pts`;
    el.tickerHits.textContent = hits;
  }

  function escapeHtml(str) {
    if (!str) return '';
    return str
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#039;');
  }

  window.addEventListener('DOMContentLoaded', init);
})();
