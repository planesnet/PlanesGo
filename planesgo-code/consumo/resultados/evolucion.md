# Evolución del consumo

Medianas de cada ronda (`<fecha>.jsonl`: los datos originales). Tokens = entrada (con la caché) + salida.

## Rondas con nota

- **2026-10-22-150558**: Punto 1: perfil de herramientas por modo (--tools): 14 herramientas propias en editar/auto y 12 en lectura, en vez de 32

## 1 · Saludo

| Agente | Modo | Ronda | Tokens | Entrada | Salida | Llamadas | Coste | Tiempo | Aciertos |
|---|---|---|--:|--:|--:|--:|--:|--:|--:|
| antigravity | sin pgocode | 2026-10-22-134725 | 18.3k | 18.3k | 5 | 1 | — | 9 s | 1/1 |
| antigravity | sin pgocode | 2026-10-22-135316 | 18.4k | 18.3k | 5 | 1 | — | 7 s | 3/3 |
| antigravity | pgocode | 2026-10-22-134725 | 18.4k | 18.4k | 5 | 1 | — | 8 s | 1/1 |
| antigravity | pgocode | 2026-10-22-135316 | 18.4k | 18.4k | 33 | 1 | — | 5 s | 3/3 |
| claude-code | sin pgocode | 2026-10-22-134725 | 23.2k | 23.2k | 4 | 1 | 0.097 $ | 4 s | 1/1 |
| claude-code | sin pgocode | 2026-10-22-135316 | 23.2k | 23.2k | 4 | 1 | 0.097 $ | 5 s | 3/3 |
| claude-code | pgocode | 2026-10-22-134725 | 19.7k | 19.7k | 4 | 1 | 0.070 $ | 5 s | 1/1 |
| claude-code | pgocode | 2026-10-22-135316 | 19.7k | 19.7k | 4 | 1 | 0.070 $ | 3 s | 3/3 |
| claude-code | pgocode | 2026-10-22-150558 | 13.7k | 13.7k | 4 | 1 | 0.068 $ | 3 s | 3/3 |
| opencode | sin pgocode | 2026-10-22-134725 | 14.0k | 14.0k | 26 | 1 | 0.028 $ | 6 s | 1/1 |
| opencode | sin pgocode | 2026-10-22-135316 | 14.0k | 14.0k | 37 | 1 | 0.028 $ | 5 s | 3/3 |
| opencode | pgocode | 2026-10-22-134725 | 14.0k | 13.9k | 35 | 1 | 0.028 $ | 6 s | 1/1 |
| opencode | pgocode | 2026-10-22-135316 | 14.0k | 13.9k | 32 | 1 | 0.028 $ | 6 s | 3/3 |

## 2 · Lectura

| Agente | Modo | Ronda | Tokens | Entrada | Salida | Llamadas | Coste | Tiempo | Aciertos |
|---|---|---|--:|--:|--:|--:|--:|--:|--:|
| antigravity | sin pgocode | 2026-10-22-134756 | 37.4k | 37.0k | 328 | 2 | — | 10 s | 1/1 |
| antigravity | sin pgocode | 2026-10-22-135316 | 37.2k | 37.0k | 234 | 2 | — | 12 s | 3/3 |
| antigravity | pgocode | 2026-10-22-134756 | 37.3k | 37.1k | 246 | 2 | — | 11 s | 1/1 |
| antigravity | pgocode | 2026-10-22-135316 | 37.3k | 37.1k | 180 | 2 | — | 13 s | 3/3 |
| claude-code | sin pgocode | 2026-10-22-134756 | 46.7k | 46.6k | 101 | 2 | 0.105 $ | 6 s | 1/1 |
| claude-code | sin pgocode | 2026-10-22-135316 | 46.7k | 46.6k | 110 | 2 | 0.105 $ | 6 s | 3/3 |
| claude-code | pgocode | 2026-10-22-134756 | 39.9k | 39.8k | 103 | 2 | 0.077 $ | 4 s | 1/1 |
| claude-code | pgocode | 2026-10-22-135316 | 39.9k | 39.8k | 103 | 2 | 0.077 $ | 4 s | 3/3 |
| claude-code | pgocode | 2026-10-22-150558 | 27.7k | 27.6k | 103 | 2 | 0.074 $ | 5 s | 3/3 |
| opencode | sin pgocode | 2026-10-22-134756 | 28.5k | 28.3k | 181 | 2 | 0.036 $ | 10 s | 1/1 |
| opencode | sin pgocode | 2026-10-22-135316 | 28.5k | 28.3k | 187 | 2 | 0.036 $ | 15 s | 3/3 |
| opencode | pgocode | 2026-10-22-134756 | 28.6k | 28.3k | 250 | 2 | 0.037 $ | 11 s | 1/1 |
| opencode | pgocode | 2026-10-22-135316 | 28.5k | 28.3k | 193 | 2 | 0.036 $ | 11 s | 3/3 |

## 3 · Búsqueda

| Agente | Modo | Ronda | Tokens | Entrada | Salida | Llamadas | Coste | Tiempo | Aciertos |
|---|---|---|--:|--:|--:|--:|--:|--:|--:|
| antigravity | sin pgocode | 2026-10-22-134756 | 57.5k | 57.2k | 289 | 3 | — | 14 s | 1/1 |
| antigravity | sin pgocode | 2026-10-22-135316 | 57.0k | 56.8k | 220 | 3 | — | 16 s | 3/3 |
| antigravity | pgocode | 2026-10-22-134756 | 57.2k | 57.0k | 219 | 3 | — | 15 s | 1/1 |
| antigravity | pgocode | 2026-10-22-135316 | 79.9k | 78.1k | 1.7k | 4 | — | 25 s | 3/3 |
| claude-code | sin pgocode | 2026-10-22-134756 | 75.3k | 74.9k | 381 | 3 | 0.129 $ | 10 s | 1/1 |
| claude-code | sin pgocode | 2026-10-22-135316 | 72.4k | 71.8k | 592 | 3 | 0.127 $ | 12 s | 3/3 |
| claude-code | pgocode | 2026-10-22-134756 | 61.9k | 61.5k | 383 | 3 | 0.094 $ | 8 s | 1/1 |
| claude-code | pgocode | 2026-10-22-135316 | 64.9k | 64.4k | 426 | 3 | 0.101 $ | 8 s | 3/3 |
| claude-code | pgocode | 2026-10-22-150558 | 44.0k | 43.4k | 544 | 3 | 0.093 $ | 10 s | 3/3 |
| opencode | sin pgocode | 2026-10-22-134756 | 65.2k | 62.8k | 2.4k | 4 | 0.074 $ | 44 s | 1/1 |
| opencode | sin pgocode | 2026-10-22-135316 | 64.0k | 62.7k | 1.2k | 4 | 0.073 $ | 42 s | 3/3 |
| opencode | pgocode | 2026-10-22-134756 | 66.5k | 64.7k | 1.8k | 4 | 0.068 $ | 39 s | 1/1 |
| opencode | pgocode | 2026-10-22-135316 | 82.2k | 80.7k | 1.5k | 5 | 0.073 $ | 38 s | 3/3 |

## 4 · Arreglo

| Agente | Modo | Ronda | Tokens | Entrada | Salida | Llamadas | Coste | Tiempo | Aciertos |
|---|---|---|--:|--:|--:|--:|--:|--:|--:|
| antigravity | sin pgocode | 2026-10-22-134756 | 73.5k | 73.1k | 375 | 4 | — | 21 s | 0/1 |
| antigravity | sin pgocode | 2026-10-22-135316 | 113.3k | 112.3k | 1.1k | 6 | — | 41 s | 3/3 |
| antigravity | pgocode | 2026-10-22-134756 | 74.2k | 73.5k | 711 | 4 | — | 22 s | 0/1 |
| antigravity | pgocode | 2026-10-22-135316 | 113.7k | 112.6k | 1.1k | 6 | — | 39 s | 3/3 |
| claude-code | sin pgocode | 2026-10-22-134756 | 99.4k | 98.6k | 821 | 4 | 0.143 $ | 15 s | 1/1 |
| claude-code | sin pgocode | 2026-10-22-135316 | 99.8k | 98.8k | 957 | 4 | 0.146 $ | 15 s | 3/3 |
| claude-code | pgocode | 2026-10-22-134756 | 85.7k | 84.8k | 967 | 4 | 0.115 $ | 18 s | 1/1 |
| claude-code | pgocode | 2026-10-22-135316 | 85.8k | 84.8k | 1.0k | 4 | 0.117 $ | 16 s | 3/3 |
| claude-code | pgocode | 2026-10-22-150558 | 64.4k | 63.3k | 1.0k | 4 | 0.116 $ | 16 s | 3/3 |
| opencode | sin pgocode | 2026-10-22-134756 | 91.1k | 90.2k | 886 | 5 | 0.069 $ | 35 s | 1/1 |
| opencode | sin pgocode | 2026-10-22-135316 | 113.5k | 112.4k | 1.0k | 6 | 0.078 $ | 41 s | 3/3 |
| opencode | pgocode | 2026-10-22-134756 | 132.9k | 131.8k | 1.2k | 7 | 0.087 $ | 39 s | 1/1 |
| opencode | pgocode | 2026-10-22-135316 | 114.2k | 112.9k | 1.3k | 6 | 0.080 $ | 48 s | 3/3 |

## 5 · Conversación

| Agente | Modo | Ronda | Tokens | Entrada | Salida | Llamadas | Coste | Tiempo | Aciertos |
|---|---|---|--:|--:|--:|--:|--:|--:|--:|
| antigravity | sin pgocode | 2026-10-22-134756 | 78.6k | 77.7k | 908 | 4 | — | 24 s | 0/1 |
| antigravity | sin pgocode | 2026-10-22-135316 | 196.3k | 194.3k | 2.0k | 10 | — | 83 s | 3/3 |
| antigravity | pgocode | 2026-10-22-134756 | 74.2k | 73.5k | 681 | 4 | — | 29 s | 0/1 |
| antigravity | pgocode | 2026-10-22-135316 | 197.2k | 195.1k | 2.1k | 10 | — | 101 s | 3/3 |
| claude-code | sin pgocode | 2026-10-22-134756 | 496.6k | 492.9k | 3.7k | 8 | 0.586 $ | 30 s | 1/1 |
| claude-code | sin pgocode | 2026-10-22-135316 | 431.5k | 427.5k | 4.1k | 7 | 0.542 $ | 29 s | 3/3 |
| claude-code | pgocode | 2026-10-22-134756 | 373.6k | 369.4k | 4.1k | 7 | 0.449 $ | 27 s | 1/1 |
| claude-code | pgocode | 2026-10-22-135316 | 376.5k | 372.2k | 4.3k | 7 | 0.462 $ | 29 s | 3/3 |
| claude-code | pgocode | 2026-10-22-150558 | 284.1k | 280.3k | 3.8k | 7 | 0.435 $ | 28 s | 3/3 |
| opencode | sin pgocode | 2026-10-22-134756 | 170.1k | 168.6k | 1.5k | 9 | 0.100 $ | 55 s | 1/1 |
| opencode | sin pgocode | 2026-10-22-135316 | 196.7k | 194.9k | 1.8k | 10 | 0.106 $ | 115 s | 3/3 |
| opencode | pgocode | 2026-10-22-134756 | 196.3k | 194.4k | 1.9k | 10 | 0.112 $ | 62 s | 1/1 |
| opencode | pgocode | 2026-10-22-135316 | 194.9k | 193.3k | 1.6k | 10 | 0.109 $ | 73 s | 3/3 |
