/**
 * useMXBrownSound — Cherry MX Brown mechanical keyboard click sound
 *
 * Synthesizes the characteristic MX Brown:
 *  - Tactile bump: short low-frequency thump (the stem actuating)
 *  - Thin metallic click: brief high-frequency snap (spring release)
 * All generated via Web Audio API — zero external files needed.
 */

import { useCallback, useRef } from 'react';

export function useMXBrownSound() {
    const ctxRef = useRef<AudioContext | null>(null);

    const getCtx = useCallback(() => {
        if (!ctxRef.current || ctxRef.current.state === 'closed') {
            ctxRef.current = new AudioContext();
        }
        // Resume if suspended (browser autoplay policy)
        if (ctxRef.current.state === 'suspended') {
            ctxRef.current.resume();
        }
        return ctxRef.current;
    }, []);

    const playClick = useCallback(() => {
        try {
            const ctx = getCtx();
            const now = ctx.currentTime;

            // ── Layer 1: Tactile bump (low thump) ──
            // White noise burst shaped with an extremely fast envelope
            const bufSize = ctx.sampleRate * 0.015; // 15ms of noise
            const noiseBuffer = ctx.createBuffer(1, bufSize, ctx.sampleRate);
            const noiseData = noiseBuffer.getChannelData(0);
            for (let i = 0; i < bufSize; i++) {
                noiseData[i] = (Math.random() * 2 - 1);
            }

            const noiseSource = ctx.createBufferSource();
            noiseSource.buffer = noiseBuffer;

            // Filter to low-mid range for the "thock" character
            const bumpFilter = ctx.createBiquadFilter();
            bumpFilter.type = 'bandpass';
            bumpFilter.frequency.value = 180;
            bumpFilter.Q.value = 0.8;

            const bumpGain = ctx.createGain();
            bumpGain.gain.setValueAtTime(0.35, now);
            bumpGain.gain.exponentialRampToValueAtTime(0.001, now + 0.018);

            noiseSource.connect(bumpFilter);
            bumpFilter.connect(bumpGain);
            bumpGain.connect(ctx.destination);
            noiseSource.start(now);
            noiseSource.stop(now + 0.018);

            // ── Layer 2: Spring click (high-freq metallic snap) ──
            // Sine oscillator at click frequency with ultra-fast decay
            const clickOsc = ctx.createOscillator();
            clickOsc.type = 'sine';
            clickOsc.frequency.setValueAtTime(1200, now);
            clickOsc.frequency.exponentialRampToValueAtTime(800, now + 0.008);

            const clickGain = ctx.createGain();
            clickGain.gain.setValueAtTime(0.15, now);
            clickGain.gain.exponentialRampToValueAtTime(0.001, now + 0.012);

            // Subtle distortion for the "crunch"
            const waveShaper = ctx.createWaveShaper();
            const curve = new Float32Array(256);
            for (let i = 0; i < 256; i++) {
                const x = (i * 2) / 256 - 1;
                curve[i] = (Math.PI + 80) * x / (Math.PI + 80 * Math.abs(x));
            }
            waveShaper.curve = curve;

            clickOsc.connect(waveShaper);
            waveShaper.connect(clickGain);
            clickGain.connect(ctx.destination);
            clickOsc.start(now);
            clickOsc.stop(now + 0.012);

            // ── Layer 3: Body resonance (very subtle low-end bloom) ──
            const resonanceOsc = ctx.createOscillator();
            resonanceOsc.type = 'sine';
            resonanceOsc.frequency.value = 95;

            const resonanceGain = ctx.createGain();
            resonanceGain.gain.setValueAtTime(0.08, now);
            resonanceGain.gain.exponentialRampToValueAtTime(0.001, now + 0.025);

            resonanceOsc.connect(resonanceGain);
            resonanceGain.connect(ctx.destination);
            resonanceOsc.start(now);
            resonanceOsc.stop(now + 0.025);

        } catch {
            // Audio not available (e.g., SSR or restricted browser) — silent fail
        }
    }, [getCtx]);

    return { playClick };
}
