# bgorch API `v1alpha1`

O schema `v1alpha1` segue o contrato de duas camadas:

1. **Camada comum portátil** (`spec.*`): runtime, nodePools, workloads, volumes, arquivos, probes e políticas genéricas.
2. **Camada de extensão tipada** (`pluginConfig`): extensões por plugin/família sem poluir o core.

## Arquivos

- JSON Schema formal: `docs/schema/v1alpha1.schema.json`
- Tipos Go: `internal/api/v1alpha1/types.go`

## Convenções principais

- `apiVersion` fixo: `bgorch.io/v1alpha1`
- `kind` fixo: `ChainCluster`
- `spec.plugin` seleciona o plugin de família (MVP: `generic-process`)
- `spec.runtime.backend` seleciona o backend de execução (MVP: `docker-compose`)
- `spec.nodePools[*].template.workloads[]` modela múltiplos processos por nó lógico
- `pluginConfig` é tipado por plugin (MVP: `genericProcess`)

## Compatibilidade futura

- Campos de backend/plugin específicos ficam em `backendConfig` e `pluginConfig`.
- Evoluções incompatíveis devem entrar em `v1beta1` ou `v1`.
- Mudanças compatíveis seguem semântica additive-only no `v1alpha1`.
