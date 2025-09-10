# Auto-Generate HTML5 Games: System Blueprint

## 1) Architecture Overview

Goal: Minimize manual prompts, maximize quality via loop: generate -> run -> verify -> critique -> repair until PASSED.

Agents:

1) Spec Planner - Produce GameSpec JSON (story, mechanics, controls, win/lose, asset list, scoring rubric) from a short brief.
2) Code Generator - Generate TypeScript/HTML/CSS via Vite template. Include loop, input, collision, score, responsive, audio.
3) Asset Generator (optional) - Produce placeholder SVG/bleep to avoid licensing issues.
4) Runner - Build & launch headless (Playwright/Chromium). Collect console logs, metrics, screenshots/recordings.
5) Verifier - Lint/type/test/UI/perf/gameplay, compute score 0-100 + PASS/REPAIR.
6) Critic/Repair - Read failure logs + rubric, output minimal patch (unified diff), repeat until PASS.
7) Packager - Package /dist, create README.md, how-to-embed, iframe snippet, cover.png, score.json.

Stack:

- Backend (Go): Orchestrator + job queue (Redis/Faktory), REST/gRPC.
- Workers: Docker (Node 20 + Playwright + pnpm + Vite + ESLint + Vitest + Lighthouse-CI).
- DB: Postgres (jobs, specs, runs, scores, artifacts index).
- Object Storage: S3/MinIO (artifacts: zips, screenshots, videos).
- Frontend (Vue/Nuxt): Dashboard preview (iframe + logs + metrics).
- Observability: Prometheus/Grafana + Loki/Elastic for logs.

## 2) GameSpec JSON (normalized)

```json
{
  "title": "Dodge Rush",
  "genre": "arcade",
  "duration_sec": 60,
  "platform": ["mobile", "desktop"],
  "controls": ["tap", "arrow_keys"],
  "mechanics": [
    {"rule": "player moves left/right"},
    {"rule": "avoid falling obstacles"},
    {"rule": "score +1 per second alive"}
  ],
  "win_condition": "survive full duration",
  "lose_condition": "collision with obstacle",
  "assets": {"sprites": "minimal SVG", "audio": "bleep sfx"},
  "constraints": {"bundle_kb_max": 600, "fps_min": 50, "no_third_party_tracking": true},
  "accessibility": {"high_contrast": true, "pause": true, "no rapid flashing": true},
  "scoring_rubric_weights": {
    "build": 0.2, "perf": 0.25, "controls": 0.2, "gameplay": 0.25, "a11y": 0.1
  }
}
```

## 3) Verify Rubric (PASS/REPAIR)

A. Build & Quality (20%)

- ESLint + TypeScript: errors = fail; warnings <= X.
- Vitest >= 80% for core logic.
- Bundle size <= bundle_kb_max.

B. Performance (25%)

- Playwright measures RAF-based FPS. FPS >= fps_min 95% of time.
- Lighthouse-CI Performance >= 85 (mobile).

C. Controls & UX (20%)

- Scripts: arrow left/right, tap/drag; restart/pause; audio toggle.

D. Gameplay (25%)

- Bot runs 10 times (15s): has losses and survivals (not too easy/hard).
- Collisions trigger; score increases; reset is correct.

E. Accessibility (10%)

- High contrast; no flashing >3Hz; focus ring; alt text.

PASS threshold: total >= 85 and no critical (build fail, broken controls, crash).

## 4) Orchestration Loop

1. Spec Planner -> GameSpec JSON
2. Code Generator -> repo scaffold

```
/game
 ├─ src/{main.ts, engine.ts, scene.ts, input.ts, audio.ts}
 ├─ index.html
 ├─ styles.css
 ├─ vite.config.ts
 ├─ tests/*.test.ts
 ├─ assets/{svg/*, sfx/*}
 └─ README.md
```

3. Runner: pnpm i && pnpm build && pnpm preview + Playwright
4. Verifier: compute score and logs
5. Critic/Repair: output unified diff patch
6. Repeat 3->5 until PASS or maxLoops

## 5) Core Prompts (short)

Spec Planner (system)

```
You are a senior game designer. Produce a strict JSON GameSpec from a short brief.
Mobile-first, minimal assets, accessible.
```

Code Generator (system)

```
You are a senior TypeScript game dev. Generate a minimal, performant HTML5 game
per the GameSpec. Use Vite + TS. Include tests for scoring/collision. Keep bundle small.
Return full file tree and file contents.
```

Critic/Repair (system)

```
You are a code reviewer. Given Verifier logs and code tree, output a minimal
unified diff patch (git apply format) to fix failures and improve the score.
Only the diff, no commentary.
```

## 6) Scripts & Tools

package.json

```json
{
  "scripts": {
    "dev": "vite",
    "build": "vite build",
    "preview": "vite preview --strictPort --port 4173",
    "lint": "eslint . --ext .ts,.tsx",
    "test": "vitest run",
    "lighthouse": "lhci autorun"
  }
}
```

Playwright specs (ideas):

- controls.spec.ts: ArrowLeft/Right & touch.
- gameplay.spec.ts: 15s, score increases, no crash.
- perf.spec.ts: RAF-based FPS approximation.
- Take cover.png (390x844) at 10s.

## 7) Orchestrator (Go) - outline

- API: POST /jobs, GET /jobs/:id
- States: QUEUED -> GENERATING -> RUNNING -> VERIFYING -> REPAIRING (loop) -> PASSED/FAILED
- Artifacts: zip dist, screenshots, score.json, report.md

## 8) Seed templates

Arcade dodge, Endless runner, Brick breaker, Memory match, 2048-lite, Flappy-like.
Reuse template + mutate mechanics to raise PASS rate.

## 9) Guardrails

- CSP, no external network.
- Asset limits (SVG inline or small PNG).
- Random seedable for deterministic tests.
- Build/run timeouts.
- Retry/backoff per error type.
- Cache node_modules and Playwright browsers; use PNPM.

## 10) Acceptance Test example

```ts
import { computeScore } from '../src/engine/score';

test('score increases ~1 per second', () => {
  let s = 0;
  for (let i=0;i<10;i++) s = computeScore(s, 1000);
  expect(s).toBeGreaterThanOrEqual(9);
  expect(s).toBeLessThanOrEqual(11);
});
```

## 11) Stop Conditions & Cost

- targetScore = 85; maxLoops = 5.
- Build fails twice -> fallback template.
- Cache & PNPM to save cost/time.

## 12) 2-week expansion (optional)

- W1: Orchestrator + Worker + 2 templates + Playwright + Rubric A/B/C.
- W2: Rubric D/E + LHCI + Repair loop + Packager + S3 + 5 templates + telemetry.
