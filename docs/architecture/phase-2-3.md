# bgorch: Arquitetura e Maturidade (Fase 2/3)

Status deste documento: **estado atual do repositório em 2026-04-16**.

## Escopo arquitetural

O projeto está estruturado em três eixos independentes:

1. **Core declarativo** (Go):
   - parse/load de spec versionada,
   - validação semântica,
   - normalização por plugin,
   - render de desired state,
   - planning por diff contra snapshot local.
2. **Plugin de chain family**:
   - validação e normalização específicas,
   - produção de artefatos específicos sem contaminar o core.
3. **Backend de runtime**:
   - validação de target/runtime,
   - tradução de desired state para artefatos executáveis do ambiente alvo.

## Maturidade por capacidade

| Capacidade | Estado | Observação |
|------------|--------|------------|
| API `v1alpha1` | Implementado | Schema comum + `pluginConfig` tipado |
| CLI `validate` | Implementado | Com saída texto/json |
| CLI `render` | Implementado | Gera artefatos determinísticos |
| CLI `plan` | Implementado | Diff contra snapshot local |
| CLI `apply` | Implementado (MVP) | Escreve artefatos + snapshot local sob lock |
| CLI `status` | Implementado (MVP) | Resume convergência por desired vs snapshot local |
| CLI `doctor` | Implementado (MVP) | Checks de spec/resolução/state + drift local |
| Plugin `generic-process` | Implementado | Base para chain desconhecida |
| Plugin real de referência | Planejado | Fase 3 |
| Backend `docker-compose` | Implementado | Render de compose + artefatos plugin |
| Backend `ssh+systemd` | Implementado (MVP) | Workloads host + render de unit files/env/layout |
| Backend `kubernetes` | Planejado | Pós-MVP |
| Adapter Terraform/Ansible | Planejado | Pós-MVP inicial |

## Fluxo atual do core

1. CLI carrega spec (`apiVersion: bgorch.io/v1alpha1`).
2. Core resolve `plugin` e `backend` via registries.
3. Executa validação:
   - validação de schema/domínio,
   - validação de plugin,
   - validação de backend target.
4. Normaliza spec no plugin.
5. Plugin produz `Output` (artefatos/metadata específicos).
6. Backend transforma em `DesiredState` e artefatos de runtime.
7. `render` persiste artefatos; opcionalmente salva snapshot.
8. `plan` compara desired com snapshot local e gera mudanças (`create/update/delete/noop`).

## Fronteiras (não negociáveis)

- Core **não** carrega detalhes de flags/config específicas de chain.
- Plugin **não** decide como runtime é materializado (Compose/systemd/K8s).
- Backend **não** modela semântica de protocolo blockchain.
- Terraform/Ansible (quando adicionados) ficam como adapters especializados, não como control plane principal.

## Lacunas conhecidas para Fase 3

- `apply` ainda não executa mutações remotas de runtime; no MVP aplica artefatos/snapshot local.
- Sem observação de runtime real (estado atual usa desired vs snapshot local).
- `doctor` ainda não faz probes remotas por backend.
- Sem rollback/repair operacional.
- Backend host (`ssh+systemd`) ainda não cobre bootstrap remoto, cópia e `systemctl` remoto.

## Referências de decisão

- `docs/adr/0001-core-architecture.md`
- `docs/adr/0002-plugin-model.md`
- `docs/adr/0003-backend-model.md`
- `docs/adr/0004-mvp-locking-and-command-semantics.md`
