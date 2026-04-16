# ADR 0004: MVP Locking e Semântica de `apply`/`status`/`doctor`

- Status: Accepted
- Date: 2026-04-16

## Context

`bgorch` já possui `validate`, `render` e `plan`, com estado local em snapshot.  
A próxima etapa adiciona mutação de runtime (`apply`) e inspeção operacional (`status`/`doctor`).

Sem um modelo de lock e semântica explícita, há risco de:
- operações concorrentes perigosas sobre o mesmo cluster,
- snapshot inconsistente,
- diagnóstico não confiável após falha parcial.

## Decision

Definir semântica MVP para os comandos futuros:

1. **Escopo de lock local**
   - Lock por par `(clusterName, backend)`.
   - Caminho canônico MVP: `.bgorch/state/<cluster>--<backend>.lock`.
   - `apply` requer lock exclusivo.
   - `status` e `doctor` são read-only e não bloqueiam por padrão.

2. **Semântica de `apply` (MVP atual)**
   - `apply` executa pipeline determinístico:
     1) load + validate + normalize,  
     2) build desired,  
     3) load snapshot current,  
     4) plan,  
     5) render de artefatos em disco,  
     6) persistência de snapshot.
   - Em falha: comando retorna erro e snapshot não é persistido.

3. **Semântica de `status`**
   - Expor convergência a partir de:
     - último snapshot aplicado conhecido,
     - observação do backend (quando disponível),
     - diferenças relevantes e estado de saúde agregado.

4. **Semântica de `doctor`**
   - Executar checks acionáveis em três classes:
     - config/spec,
     - runtime/backend,
     - saúde/probes/artefatos.
   - Sem mutação de estado remoto.

5. **Persistência mínima recomendada**
   - `last-applied.json` (snapshot consistente),
   - `last-attempt.json` (resultado da última tentativa),
   - `events.log` (eventos operacionais, sem segredos).

## Rationale

- Preserva idempotência e segurança operacional com baixo custo de implementação no MVP.
- Evita corridas entre múltiplos `apply` locais no mesmo alvo.
- Mantém trilha de auditoria básica para troubleshooting.

## Consequences

### Positivas
- Reduz risco de corrupção de estado local.
- Cria base limpa para `status` e `doctor` confiáveis.
- Mantém compatibilidade com evolução futura para lock distribuído.

### Negativas
- Lock local não resolve concorrência entre múltiplas máquinas diferentes.
- Exige disciplina de persistência atômica de arquivos de estado.

## Out of scope (MVP)

- Lock distribuído (Postgres/etcd/Redis).
- Orquestração transacional cross-backend.
- Execução remota real no backend (`docker compose up`, `systemctl`, SSH copy/exec).
- Rollback automático full-fidelity para todos os plugins/backends.

## Rejected alternatives

1. **Sem lock no `apply`**
   - Rejeitado: risco alto de concorrência destrutiva.
2. **Lock global único para todo o projeto**
   - Rejeitado: serializa operações independentes sem necessidade.
3. **`status`/`doctor` com lock exclusivo**
   - Rejeitado: reduz observabilidade durante manutenção sem ganho proporcional.

## Implementation note

`apply`/`status`/`doctor` já estão expostos no CLI em modo MVP local-state-first.
Evolução seguinte: incorporar `Observe/Apply/Verify` remotos por backend mantendo as mesmas semânticas de segurança.
