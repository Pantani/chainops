# Guia Operacional (MVP): Comandos e Integração Plugin/Backend

Este guia separa explicitamente o que está **implementado** do que está **planejado**.

## Comandos

| Comando | Estado | Objetivo | Efeito colateral |
|---------|--------|----------|------------------|
| `validate -f <spec>` | Implementado | Validar schema/domínio/plugin/backend | Nenhum |
| `render -f <spec> -o <dir>` | Implementado | Gerar artefatos determinísticos do desired state | Escreve arquivos em `<dir>` |
| `render ... --write-state` | Implementado | Mesmo do `render` + persistir snapshot local | Escreve arquivos + snapshot em `.bgorch/state` |
| `plan -f <spec>` | Implementado | Diff entre desired e snapshot local atual | Nenhum |
| `apply -f <spec>` | Implementado (MVP) | Executar reconciliação local com lock | Escreve artefatos em `<dir>` e snapshot em `.bgorch/state` |
| `status -f <spec>` | Implementado (MVP) | Expor convergência desired vs snapshot local | Leitura de spec/snapshot |
| `doctor -f <spec>` | Implementado (MVP) | Diagnóstico de spec/resolução/state/drift local | Leitura de spec/snapshot |

## Execução segura no estado atual

Fluxo recomendado hoje:

```bash
go run ./cmd/bgorch validate -f examples/generic-single-compose.yaml
go run ./cmd/bgorch apply -f examples/generic-single-compose.yaml -o .bgorch/render --dry-run
go run ./cmd/bgorch apply -f examples/generic-single-compose.yaml -o .bgorch/render
go run ./cmd/bgorch plan -f examples/generic-single-compose.yaml
go run ./cmd/bgorch status -f examples/generic-single-compose.yaml
go run ./cmd/bgorch doctor -f examples/generic-single-compose.yaml
```

Limitação importante:
- `status` e `doctor` no MVP usam snapshot local e não fazem observação remota de runtime.
- `apply` no MVP não executa `docker compose up` nem `systemctl` remoto; ele persiste artefatos + snapshot com lock.

## Semântica alvo de evolução de `apply/status/doctor` (próximas fases)

Definida em `docs/adr/0004-mvp-locking-and-command-semantics.md`.

Resumo:
- `apply`: lock exclusivo por cluster/backend, execução idempotente, verificação pós-apply, persistência atômica de snapshot.
- `status`: leitura de snapshot + observação do backend, sem mutação.
- `doctor`: checks de configuração/execução/saúde, saída acionável.

## Modelo de integração Plugin/Backend

Pipeline de integração:

1. `spec` seleciona `spec.plugin` e `spec.runtime.backend`.
2. Core resolve ambos em registries.
3. Plugin valida/normaliza e gera `Output` específico.
4. Backend valida target e converte `Output` em `DesiredState`.
5. Core renderiza/plana sobre artefatos e estado.

## Contrato para novo plugin

Checklist mínimo:
- Implementar `Name`, `Family`, `Capabilities`.
- Implementar `Validate` e `Normalize`.
- Implementar `Build` produzindo artefatos/metadata determinísticos.
- Declarar campos específicos apenas em `pluginConfig` tipado.
- Não introduzir campos específicos de chain no schema comum sem ADR.

## Contrato para novo backend

Checklist mínimo:
- Implementar `Name`.
- Implementar `ValidateTarget` com diagnósticos claros.
- Implementar `BuildDesired` determinístico.
- Não mover semântica de protocolo para o backend.
- Declarar gaps de capacidade explicitamente (ex.: host mode, probes, restart nuances).

## Matriz de compatibilidade (estado atual)

| Plugin | Backend | Estado |
|--------|---------|--------|
| `generic-process` | `docker-compose` | Implementado |
| `generic-process` | `ssh-systemd` | Implementado (MVP host-only) |

## Observabilidade operacional (MVP)

- Saída de diagnósticos em texto e JSON (`validate`/`plan`).
- Saída determinística de artefatos para revisão/auditoria.
- Sem telemetria runtime integrada ainda (planejado para fases seguintes).
