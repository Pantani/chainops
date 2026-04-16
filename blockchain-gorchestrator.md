# Prompt para Codex — Orquestrador Declarativo Multi-Blockchain em Go

Você é um **principal engineer de plataforma**, com foco em **distributed systems**, **developer tooling**, **infraestrutura por código**, **orquestração**, **containers** e **operações de blockchains**.

Sua missão é **projetar e iniciar a implementação** de um projeto open-source chamado **`bgorch`** (nome provisório), um **orquestrador declarativo multi-blockchain** para **deploy**, **configuração**, **bootstrap**, **upgrade**, **backup/restore**, **observabilidade** e **lifecycle management** de nós e clusters de blockchain.

## Contexto do produto

Eu quero um produto que funcione para **qualquer blockchain**, e não apenas para Cosmos, Ethereum, Bitcoin etc.

Isso significa que o design **não pode acoplar o core** a uma única família de chain. Em vez disso, o sistema deve ter:

1. um **core genérico** de orquestração;
2. uma **camada comum** para conceitos cross-chain;
3. **plugins/adapters por família de blockchain**;
4. **profiles por chain específica**;
5. **backends de execução** independentes da chain.

## Diretriz principal de linguagem

- **Preferir Go como linguagem principal**.
- Só escolha outra linguagem para algum componente se houver uma **justificativa técnica forte e explícita**.
- Mesmo que algum subcomponente use outra linguagem, o **core/control plane/CLI principal** deve permanecer em **Go**.
- Documente qualquer exceção em ADR.

## O que NÃO quero

- Não quero começar como “um provider de Terraform” apenas.
- Não quero começar como “uma collection de Ansible” apenas.
- Não quero um projeto preso a uma chain específica.
- Não quero um wrapper raso de Docker Compose.
- Não quero um projeto que só faça `docker run` com templates soltos.
- Não quero um design que assuma que toda chain é um único processo.
- Não quero um design que assuma Kubernetes como único runtime.
- Não quero um design em que todo detalhe específico de chain vaze para o core.

## O que eu quero

Quero um **core declarativo próprio**, em Go, com **desired state**, **plan/apply**, **reconciliação idempotente**, **detecção de drift**, **renderização determinística de configuração** e **adapters/backends** para diferentes ambientes.

Quero que você trate:

- **Terraform** como um possível **adapter de infra/provisionamento**;
- **Ansible** como um possível **adapter de bootstrap/configuração de hosts**;
- **Dockerfile** como mecanismo de empacotamento;
- **Docker Compose** como um backend/orchestrator local ou simples;
- **Kubernetes** como um backend avançado para workloads stateful;
- **SSH + systemd** como backend importante para bare metal/VMs;
- e que o produto possa crescer para outros alvos no futuro.

## Estratégia de arquitetura

Projete o sistema com estas camadas:

### 1. Core declarativo
Responsável por:
- ler specs YAML/JSON;
- validar schema + regras;
- calcular plano;
- reconciliar desired state vs current state;
- aplicar mudanças de forma idempotente;
- gerar outputs, status, eventos e diagnósticos.

### 2. Modelo de domínio comum
Conceitos genéricos que existem em várias chains:
- cluster
- node
- node set / node pool
- role
- workload
- process set
- data directory
- config files
- storage
- secrets
- sync/bootstrap strategy
- snapshot/backup policy
- restart policy
- health checks
- upgrade strategy
- observability
- networking
- resource sizing
- lifecycle hooks

### 3. Extensões por família de blockchain
Cada família deve poder:
- validar campos específicos;
- renderizar arquivos de configuração;
- declarar processos necessários;
- definir estratégias de sync/bootstrap;
- definir health checks;
- declarar compatibilidades e limitações;
- expor extensões sem contaminar o core.

### 4. Backends de execução
Comece com uma interface clara para suportar:
- Docker Engine
- Docker Compose
- SSH + systemd
- Kubernetes
- adapter de Ansible
- adapter de Terraform

### 5. Estado e reconciliação
O sistema deve ter um mecanismo claro para:
- desired state
- observed state
- diff
- plan
- apply
- verify
- rollback/repair
- locks e segurança contra operações concorrentes perigosas

## Primeiro passo obrigatório: pesquisa

Antes de implementar o core, faça uma **pesquisa guiada em documentação oficial** e produza um documento em:

`docs/research/infra-foundations.md`

Esse documento deve resumir **boas práticas e implicações de design** para o projeto a partir de:

1. Dockerfile / Docker Build
2. Docker Compose
3. Ansible
4. Terraform
5. Kubernetes controllers/operators
6. Kubernetes para workloads stateful
7. Go modules / organização de projeto em Go

### Regras da pesquisa

- Priorize **documentação oficial**.
- Cite links oficiais no documento.
- Para cada tecnologia, extraia:
  - o modelo mental;
  - as responsabilidades corretas daquela tecnologia;
  - boas práticas relevantes;
  - o que **não** deve ser empurrado para ela;
  - como isso influencia o design do `bgorch`.

### Resultado esperado da pesquisa

No final desse documento, crie uma seção:

`Design Implications for bgorch`

Com uma tabela neste formato:

| Tecnologia | Boa prática | Risco se usada errado | Decisão no bgorch |
|------------|-------------|-----------------------|---------------------|

## Resultado esperado do projeto

Quero que você entregue uma base de projeto que permita evoluir para um produto com estes objetivos:

- suportar **múltiplas famílias de blockchain**;
- suportar **múltiplos processos por node**;
- suportar **estado persistente**;
- suportar **bootstrap por snapshot / state sync / restore / genesis / custom**;
- suportar **nós archive, pruned, validator, RPC, sentry, full node, relayer, observer**;
- suportar **deploy local, VMs e Kubernetes**;
- suportar **plan/apply/status/doctor/render**;
- ser extensível sem precisar reescrever o core.

## Modelo de abstração desejado

Você deve desenhar o sistema em **duas camadas de schema**:

### Camada 1: schema comum e portátil
Inclui apenas conceitos que fazem sentido cross-chain.

### Camada 2: extensões específicas
Cada plugin de família pode declarar:
- campos adicionais;
- validações;
- renderização;
- processos auxiliares;
- lógica de bootstrap;
- regras de upgrade;
- observabilidade específica.

### Regra importante
Não coloque campos específicos de uma chain no schema comum sem uma justificativa forte.

Quando algo for específico demais, use:
- `familyConfig`
- `pluginConfig`
- ou mecanismo equivalente

…desde que exista **validação tipada** e não seja apenas um `map[string]any` sem controle.

## Princípios de design

1. **Go-first**
   - CLI, engine, planner, reconciler e plugins preferencialmente em Go.

2. **Idempotência**
   - Repetir `apply` não deve quebrar o ambiente.
   - O sistema deve convergir para o desired state.

3. **Determinismo**
   - Mesma spec + mesmo contexto => mesmos artefatos/renderizações.

4. **Segurança**
   - Não embutir secrets em imagem.
   - Não vazar chaves em logs.
   - Tratar validator keys / wallet keys / signing keys com extremo cuidado.
   - Permitir integração com secret stores futuramente.

5. **Drift detection**
   - Detectar divergência entre estado desejado e atual.

6. **Extensibilidade**
   - Adicionar nova família de chain deve ser um trabalho localizado.

7. **Múltiplos runtimes**
   - Containers e host binaries devem ser suportados.
   - Nem toda chain deve exigir container.

8. **Stateful by design**
   - Não tratar node stateful como se fosse app stateless.

9. **Plan first**
   - Toda mudança importante deve passar por `plan`.

10. **Observabilidade embutida**
    - Logs, métricas, status e diagnósticos desde o início.

11. **Pragmatismo**
    - MVP pequeno, arquitetura sólida.
    - Não tentar suportar todas as chains no primeiro commit.

## Casos que o design precisa suportar

O sistema deve conseguir modelar:

### Caso A — Chain simples de processo único
Um daemon único, com:
- binário ou imagem
- portas
- diretórios
- arquivo de config
- volume persistente
- restart policy
- backup policy

### Caso B — Chain multi-processo
Exemplo genérico:
- execution client
- consensus client
- validator
- sidecar/exporter/proxy

O design deve suportar um único “node lógico” composto por **vários processos/workloads**.

### Caso C — Deploy em host
Sem container. Usando:
- download de binário
- criação de diretórios
- templates
- systemd
- health checks

### Caso D — Deploy em containers
Usando:
- Dockerfile
- Docker Engine
- Docker Compose
- named volumes / bind mounts

### Caso E — Deploy em Kubernetes
Usando recursos adequados para workloads stateful, com PVCs, config e secrets.

### Caso F — Custom blockchain desconhecida
Usuário fornece:
- imagem ou binário;
- comando/args;
- templates;
- portas;
- volumes;
- probes;
- hooks;
- strategy plugins;
e o sistema ainda funciona sem precisar de suporte hardcoded.

## Direção de MVP

Para o MVP, implemente **primeiro o motor genérico** e depois **referências**, não o contrário.

### MVP obrigatório

1. **CLI em Go**
2. **Schema versionado**
3. **Validação**
4. **Render**
5. **Plan**
6. **Apply**
7. **Status**
8. **Doctor**
9. **Backend Docker Compose**
10. **Backend SSH + systemd**
11. **Plugin genérico `generic-process`**
12. **Pelo menos 1 plugin de referência real**
13. **Testes unitários e golden tests**
14. **Documentação e exemplos**

### Sugestão de plugin de referência real
Escolha um dos caminhos:
- `bitcoin-core`
- `ethereum-stack`
- `cometbft-family`
- outro, desde que justificado

Mas **não** deixe o design depender dele.

## Interface de alto nível do CLI

Projete algo como:

```bash
bgorch init
bgorch validate -f examples/generic-node.yaml
bgorch render -f examples/generic-node.yaml
bgorch plan -f examples/generic-node.yaml
bgorch apply -f examples/generic-node.yaml
bgorch status -f examples/generic-node.yaml
bgorch doctor -f examples/generic-node.yaml
bgorch backup -f examples/generic-node.yaml
bgorch restore -f examples/generic-node.yaml
```

## Modelo de recursos sugerido

Você pode ajustar os nomes, mas quero algo nessa linha:

- `ChainCluster`
- `NodePool`
- `Node`
- `WorkloadSet`
- `StoragePolicy`
- `SyncPolicy`
- `UpgradePolicy`
- `BackupPolicy`
- `ObservabilityPolicy`
- `SecretRef`
- `ChainFamily`
- `ChainProfile`
- `RuntimeBackend`

Ou um modelo equivalente, desde que bem justificado.

## Interface de plugin sugerida

Projete uma interface em Go equivalente a algo como:

```go
type ChainPlugin interface {
    Name() string
    Family() string
    Capabilities() Capabilities

    Validate(spec *ChainClusterSpec) []Diagnostic
    Normalize(spec *ChainClusterSpec) error

    RenderConfig(ctx context.Context, req RenderRequest) ([]RenderedArtifact, error)
    BuildWorkloads(ctx context.Context, req BuildRequest) ([]WorkloadSpec, error)

    HealthChecks(spec *ChainClusterSpec) []HealthCheck
    BootstrapPlan(spec *ChainClusterSpec) ([]Action, error)
    BackupPlan(spec *ChainClusterSpec) ([]Action, error)
    RestorePlan(spec *ChainClusterSpec) ([]Action, error)
    UpgradePlan(spec *ChainClusterSpec, from, to Version) ([]Action, error)
}
```

Você pode mudar a interface se encontrar desenho melhor, mas preserve a ideia central:
- validação;
- renderização;
- descrição de workloads;
- lifecycle hooks;
- estratégias de operação.

## Backend interface sugerida

Projete uma interface em Go equivalente a:

```go
type Backend interface {
    Name() string

    ValidateTarget(ctx context.Context, target Target) []Diagnostic
    Observe(ctx context.Context, scope Scope) (ObservedState, error)

    Plan(ctx context.Context, desired DesiredState, observed ObservedState) (ExecutionPlan, error)
    Apply(ctx context.Context, plan ExecutionPlan) (ApplyResult, error)
    Verify(ctx context.Context, desired DesiredState) (VerificationResult, error)
}
```

## Arquitetura interna desejada

Estruture o projeto com algo próximo de:

```text
bgorch/
  cmd/
    bgorch/
  internal/
    app/
    cli/
    domain/
    api/
    planner/
    reconcile/
    renderer/
    backend/
      compose/
      sshsystemd/
      kubernetes/
      ansible/
      terraform/
    chain/
      generic/
      bitcoin/
      ethereum/
      cometbft/
    state/
    secrets/
    observability/
    doctor/
  examples/
  docs/
    research/
    adr/
  test/
```

Não siga isso cegamente; refine se achar melhor, mas mantenha a separação de responsabilidades.

## Requisitos importantes de implementação

### 1. API versionada
- Use algo como `v1alpha1`.
- Prepare o terreno para evolução futura.

### 2. Renderização determinística
- Templates e arquivos gerados devem ser testáveis.
- Use golden tests para config render.

### 3. Estado local do projeto
- Comece simples.
- Pode usar diretório local de estado + locks.
- Abstraia a camada para futura troca por SQLite/Postgres/etcd, se necessário.

### 4. Operações seguras
- `plan` antes de `apply`;
- `--dry-run`;
- verificações pós-apply;
- mensagens claras;
- falhas parciais tratadas com cuidado.

### 5. Logs e diagnósticos
- structured logging;
- erros acionáveis;
- saída humana e, se possível, JSON.

### 6. Testes
Inclua:
- unit tests;
- golden tests;
- testes de planner;
- testes de render;
- testes mínimos de integração para o backend Compose;
- testes de validação de spec.

### 7. Examples
Entregue exemplos completos para:
- generic single-process node
- generic multi-process node
- deploy por Docker Compose
- deploy por SSH + systemd
- um plugin real de referência

## Boas práticas que quero embutidas no design

Sem se limitar a estes pontos, quero que a pesquisa e a implementação considerem:

- imagens pequenas e seguras;
- builds reproduzíveis;
- separação clara entre build e runtime;
- volumes persistentes para dados stateful;
- não guardar secrets dentro de imagens;
- idempotência;
- composição e reaproveitamento;
- validação antes de aplicar;
- separation of concerns entre infra, bootstrap e operação;
- suporte a rollback/repair;
- health checks;
- readiness / liveness / startup semantics quando aplicável;
- artefatos gerados versionáveis;
- comportamento previsível em reexecução.

## O que o design NÃO deve assumir

- que todo node usa Docker;
- que todo node usa Kubernetes;
- que toda chain usa um único banco/local state;
- que toda chain suporta pruning da mesma forma;
- que toda chain tem as mesmas flags;
- que todo upgrade é rolling;
- que todo bootstrap é via snapshot;
- que Terraform deve conhecer detalhes internos da chain;
- que Ansible deve virar o core do produto.

## Abordagem correta para “qualquer blockchain”

Modele o problema assim:

### Camada comum
Campos portáveis:
- nome
- família
- profile
- runtime/backend
- workloads
- volumes
- redes
- portas
- recursos
- arquivos renderizados
- env
- command/args
- restart policy
- sync/bootstrap category
- backup policy
- observability
- secret refs

### Camada específica
Campos por plugin:
- pruning knobs específicos
- db backend específico
- flags específicas
- config TOML/YAML/JSON específicas
- sidecars específicos
- topologias específicas
- regras de bootstrap específicas
- regras de upgrade específicas

## Backends que devem existir no design

### Backend 1 — Docker Compose
Deve:
- gerar compose file;
- modelar services, networks e volumes;
- suportar named volumes/bind mounts;
- suportar health checks/restart policies;
- subir e derrubar ambientes locais.

### Backend 2 — SSH + systemd
Deve:
- preparar diretórios;
- copiar/renderizar configs;
- instalar ou posicionar binários;
- gerar systemd units;
- iniciar/parar/reiniciar serviços;
- observar status.

### Backend 3 — Kubernetes
Mesmo que o MVP não implemente tudo, deixe a arquitetura pronta para:
- StatefulSets
- PVCs
- ConfigMaps / Secrets
- Services
- probes
- rolling updates controladas
- afinidade / anti-afinidade / tolerations quando fizer sentido

### Backend 4 — Terraform adapter
O adapter de Terraform deve focar em:
- provisionamento de infra;
- VMs;
- discos;
- redes;
- buckets;
- security groups/firewall;
- clusters Kubernetes.

Não empurre para Terraform o que deveria ser responsabilidade do reconciler de runtime.

### Backend 5 — Ansible adapter
O adapter de Ansible deve focar em:
- bootstrap de host;
- templates;
- configuração de diretórios;
- distribuição de arquivos;
- systemd;
- handlers/restarts;
- integração com inventories.

Não transforme Ansible no control plane principal.

## Segurança e chaves

Trate como requisito de primeira classe:

- chaves privadas;
- validator keys;
- wallet keys;
- mnemonics;
- tokens;
- credenciais de RPC;
- credenciais cloud.

Quero:
- abstração de `SecretRef`;
- opção para env/file/provider externo;
- nunca logar segredo;
- nunca serializar segredo à toa;
- caminho futuro para KMS/Vault/SOPS ou equivalentes.

## Observabilidade

Inclua desde o início:
- structured logs;
- health model;
- status model;
- eventos/diagnósticos;
- espaço para métricas;
- integração futura com Prometheus/Grafana/Loki ou equivalentes.

## Saída esperada do Codex

Quero que você trabalhe em fases e **não tente codar tudo de uma vez sem pensar**.

### Fase 0 — Pesquisa e arquitetura
Entregue:
1. resumo executivo do problema;
2. `docs/research/infra-foundations.md`;
3. `docs/adr/0001-core-architecture.md`;
4. `docs/adr/0002-plugin-model.md`;
5. `docs/adr/0003-backend-model.md`;
6. proposta do schema `v1alpha1`;
7. árvore inicial do repositório.

### Fase 1 — Scaffold do projeto
Entregue:
- módulo Go;
- estrutura de diretórios;
- CLI mínima;
- parser de spec;
- validação básica;
- registries de backend/plugin;
- comandos `validate`, `render`, `plan`.

### Fase 2 — MVP funcional
Entregue:
- plugin `generic-process`;
- backend `docker-compose`;
- backend `ssh+systemd`;
- planner básico;
- renderização de arquivos;
- examples;
- testes mínimos.

### Fase 3 — Referência real
Entregue:
- 1 plugin real de referência;
- exemplos reais;
- documentação de uso;
- decisões de compatibilidade.

### Fase 4 — Extensibilidade
Prepare:
- backend Kubernetes;
- adapter Terraform;
- adapter Ansible;
- API/plugin evolution.

## Regras de execução

1. **Pense antes de codar**.
2. **Comece pela pesquisa e ADRs**.
3. **Explique trade-offs**.
4. **Prefira simplicidade estrutural no MVP**.
5. **Evite abstrações mágicas demais**.
6. **Evite dependências desnecessárias**.
7. **Priorize interfaces claras e testáveis**.
8. **Não esconda problemas; documente limites**.
9. **Não finja suporte universal sem base real**.
10. **Projete para extensão, não para overengineering inicial**.

## Entrega inicial desejada nesta rodada

Nesta primeira execução, faça o seguinte:

1. Produza o **resumo arquitetural**.
2. Produza o **plano de implementação por fases**.
3. Gere a **árvore de diretórios inicial**.
4. Crie os **ADRs iniciais**.
5. Crie o **schema `v1alpha1` inicial**.
6. Faça o **scaffold em Go**.
7. Implemente:
   - `bgorch validate`
   - `bgorch render`
   - `bgorch plan`
   - registry de plugins
   - registry de backends
   - plugin `generic-process`
   - backend `docker-compose` com renderização de compose file
8. Adicione **2 exemplos completos**
9. Adicione **testes**
10. Documente claramente o que ficou como próximo passo

## Critérios de aceitação

Considerarei a entrega boa se:

- o core estiver em Go;
- o design não estiver preso a uma chain;
- o schema estiver limpo;
- houver separação clara entre core, plugins e backends;
- o backend Compose gerar artefatos úteis;
- o plugin genérico permitir modelar uma chain desconhecida;
- o projeto tiver docs, exemplos e testes;
- as ADRs explicarem por que o core não é simplesmente Terraform ou Ansible;
- der para evoluir para Kubernetes, Terraform e Ansible sem reescrever tudo.

## Formato da resposta

Quero que você responda nesta ordem:

1. **Resumo da arquitetura proposta**
2. **Principais trade-offs**
3. **Fases de implementação**
4. **Árvore do repositório**
5. **Arquivos que serão criados**
6. **Conteúdo dos ADRs**
7. **Conteúdo do scaffold inicial em Go**
8. **Exemplos**
9. **Testes**
10. **Próximos passos**

Se precisar fazer suposições, explicite.

Comece agora.
