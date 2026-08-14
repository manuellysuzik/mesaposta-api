# mesaposta-api

MesaPosta é um backend de gestão de restaurante (pedidos, mesas, cardápio,
pagamentos, etc.), construído em Go seguindo DDD tático + Arquitetura
Hexagonal, organizado como um **monólito modular multi-tenant**:

- Postgres compartilhado entre tenants, com `tenant_id` obrigatório em toda
  tabela multi-tenant e Row Level Security (RLS) no banco como segunda linha
  de defesa (além do isolamento aplicado na camada de aplicação).
- Módulos de domínio isolados dentro do monólito (`internal/modules/<nome>/`),
  cada um com sua própria camada `domain/`, expondo apenas `contracts/` para
  ser consumido por outros módulos.
- Infraestrutura compartilhada (autenticação, contexto de tenant, taxonomia
  de erros, outbox, etc.) centralizada em `internal/platform/`.

Este repositório é o próprio serviço `mesaposta-api` — não há um subdiretório
`services/api/`; a raiz do repositório é a raiz do módulo Go. Veja
[`docs/adr/0001-module-layout.md`](docs/adr/0001-module-layout.md) para o
racional dessa decisão.

## Status atual

Todas as fases do plano de arquitetura original estão implementadas: a
fundação (Fase 0) e os cinco módulos de domínio (`operations`, `catalog`,
`crm`, `community`, `ordering` — este último com um adapter público sem
autenticação para o cardápio digital via QR code). **Comece por
[`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md)** para uma visão geral e um
roteiro de leitura do código; este README cobre só o "como rodar
localmente".

O que a fundação (Fase 0) entregou, e que toda regra de negócio depois se
apoia em cima:

- O esqueleto do repositório (tooling, lint, formatação, CI gate local via
  `make check`).
- A **fitness function de arquitetura** (`architecture/`), que valida em
  tempo de `go test` que a estrutura de dependências do código respeita as
  fronteiras da arquitetura (domínio não pode depender de infraestrutura,
  módulos não podem depender uns dos outros por dentro, etc.). Veja
  [`docs/adr/0002-arch-fitness-tooling.md`](docs/adr/0002-arch-fitness-tooling.md).
- Infraestrutura compartilhada completa em `internal/platform/`: taxonomia
  de erros (`errs`), contexto de tenant (`tenant`), outbox transacional
  (`outbox`, `outbox/postgres`), autenticação (`auth`, `auth/token`,
  `auth/refresh`, `auth/password`), acesso a Postgres com Row Level Security
  obrigatória (`db`), configuração (`config`) e a camada HTTP compartilhada
  (`httpx`: envelope padrão, roteador, middlewares, health/readiness).
- A **composition root** (`cmd/api` + `internal/platform/app`), que monta
  tudo isso em um binário real, com shutdown gracioso e migrations
  opcionais no boot — veja
  [`docs/adr/0003-composition-root-and-e2e-proof-slice.md`](docs/adr/0003-composition-root-and-e2e-proof-slice.md).
- Um teste end-to-end (`internal/platform/httpx/e2e_integration_test.go`)
  que prova, contra um Postgres real, que JWT → contexto de tenant → Row
  Level Security → envelope de resposta estão corretamente costurados de
  ponta a ponta.
- CI (`.github/workflows/ci.yml`) e o gate de cobertura combinada — veja
  [`docs/adr/0004-ci-pipeline-and-coverage-gate.md`](docs/adr/0004-ci-pipeline-and-coverage-gate.md).
- O template de layout que todo módulo de domínio da Fase 1+ deve seguir:
  [`internal/modules/README.md`](internal/modules/README.md).

## Requisitos

- Go 1.24.5+
- [`golangci-lint`](https://golangci-lint.run/) instalado e no `PATH`
- `make` (GNU Make) disponível no `PATH`
- Docker (Docker Desktop no Windows/macOS, ou Docker Engine no Linux) —
  necessário apenas para os testes de integração (`-tags=integration`), que
  sobem Postgres real via [testcontainers-go](https://golang.testcontainers.org/).
  Não é necessário para `make check`.

## Como rodar localmente

### `make check` (rápido, não precisa de Docker)

```bash
make check
```

Isso executa, em sequência:

1. `gofmt -l .` — falha se algum arquivo `.go` não estiver formatado.
2. `go vet ./...`
3. `golangci-lint run` (config em [`.golangci.yml`](.golangci.yml): govet,
   staticcheck, errcheck, gosec, unused, ineffassign, gofmt, goimports)
4. `go test -race -cover -count=1 ./...`

O `-count=1` desabilita o cache de resultado de teste do Go. Isso importa
especificamente para `./architecture/...`: seus testes carregam pacotes do
disco em tempo de execução via `golang.org/x/tools/go/packages` (uma leitura
dinâmica de filesystem que o build cache do Go não rastreia como input do
teste), então um resultado `ok` em cache poderia mascarar uma violação de
arquitetura recém-introduzida em outro lugar do repositório.

Você também pode rodar cada etapa isoladamente: `make fmt`, `make vet`,
`make lint`, `make test`.

### Testes de integração (precisa de Docker rodando)

Todo teste cujo nome termina em `_integration_test.go` fica atrás da build
tag `integration` e é ignorado por `make check`/`go test ./...`. Esses
testes sobem um container Postgres efêmero por suíte via testcontainers-go
(`internal/platform/db.TestPostgres`, que também aplica as migrations reais
antes de cada suíte rodar) — não é preciso subir nada manualmente, só ter o
Docker Desktop/Engine rodando:

```bash
go test -tags=integration -race -count=1 ./...
```

Isso inclui, entre outras, a suíte end-to-end
`internal/platform/httpx/e2e_integration_test.go`, que prova o fluxo
completo JWT → tenant → RLS → envelope contra um Postgres real.

### Subindo o servidor (`cmd/api`)

```bash
cp .env.example .env   # preencha DATABASE_URL e JWT_SECRET
export $(grep -v '^#' .env | xargs)   # ou use algo como direnv/dotenv
go run ./cmd/api
```

Variáveis de ambiente: veja [`internal/platform/config/config.go`](internal/platform/config/config.go)
para a lista completa (obrigatórias: `DATABASE_URL`, `JWT_SECRET`;
opcionais, com default: `APP_ENV`, `PORT`, `DB_MAX_CONNS`, `DB_MIN_CONNS`,
`DB_MAX_CONN_LIFETIME`, `DB_MAX_CONN_IDLE_TIME`, `DB_CONNECT_TIMEOUT`).

Duas variáveis adicionais, lidas diretamente por `cmd/api` (não fazem parte
de `internal/platform/config`, por serem uma decisão de deploy, não de
configuração de runtime da aplicação):

- `MIGRATE_ON_BOOT` (default `false`): quando `true`, o processo roda as
  migrations pendentes (`internal/platform/db.Migrate`) antes de começar a
  aceitar requisições. **O caminho recomendado em produção é rodar
  migrations como um step de deploy separado** (antes de subir uma nova
  revisão do binário), não a cada boot de cada réplica — múltiplas réplicas
  subindo ao mesmo tempo com `MIGRATE_ON_BOOT=true` serializam entre si via
  advisory lock do Postgres (ver o doc comment de `db.Migrate`), mas ainda
  assim é desnecessário/arriscado repetir a migration a cada deploy quando
  um step de deploy dedicado resolve isso uma única vez. `MIGRATE_ON_BOOT`
  existe principalmente como conveniência para desenvolvimento local.
  **`cmd/api` recusa `MIGRATE_ON_BOOT=true` quando `APP_ENV=production`**
  (saída não-zero, sem tentar abrir conexão nenhuma) — isso é reforçado em
  código, não só documentado, porque `0001_baseline.sql` cria a role
  `mesaposta_app` com uma senha de placeholder de desenvolvimento que
  precisa ser rotacionada manualmente (`ALTER ROLE ... PASSWORD ...`, via
  o secret manager real) antes de qualquer uso em produção; sem essa
  guarda, rodar a migration automaticamente em produção poderia deixar
  essa senha conhecida ativa, ainda que por uma janela curta.
- `MIGRATE_DATABASE_URL` (opcional; default: o mesmo valor de
  `DATABASE_URL`): o DSN administrativo usado quando `MIGRATE_ON_BOOT=true`.
  Migrations precisam de privilégios elevados (criar roles, tabelas, etc.)
  que o papel `mesaposta_app` usado por `DATABASE_URL` deliberadamente NÃO
  tem (ver o doc comment de `internal/platform/db/migrate.go` para o porquê)
  — em produção, configure esta variável com um DSN administrativo
  separado em vez de depender do fallback.

O processo encerra de forma graciosa em `SIGINT`/`SIGTERM`: para de aceitar
conexões novas e espera requisições em andamento terminarem (até um timeout
fixo de 10s), antes de sair. Uma configuração inválida no boot imprime uma
mensagem clara em stderr e sai com código de saída diferente de zero — nunca
um stack trace de panic.

### Observabilidade local (`docker-compose up`)

`docker-compose.yml` sobe a stack completa (Postgres, `api`, Jaeger,
Prometheus e Grafana) com `OTEL_EXPORTER_OTLP_ENDPOINT` já configurado —
ver [`internal/platform/observability`](internal/platform/observability)
para o que isso habilita (traces via OTLP/HTTP + métricas Prometheus em
`/metrics`, incluindo o histograma `http.server.request.duration` usado
para p50/p95/p99).

```bash
docker compose up --build
```

- **Grafana** — <http://localhost:3000> (acesso anônimo, sem login) — abre
  direto no dashboard `mesaposta-api` (provisionado automaticamente via
  [`deploy/grafana/`](deploy/grafana)), com latência p50/p95/p99 por rota,
  taxa de erro 5xx e saturação do pool de conexões Postgres.
- **Prometheus** — <http://localhost:9090> — para consultas PromQL ad-hoc.
- **Jaeger UI** — <http://localhost:16686> — para inspecionar traces
  individuais (útil junto com o `trace_id` que aparece nos logs
  estruturados de cada requisição — ver
  [`internal/platform/httpx`](internal/platform/httpx)).

## Como verificar a fitness function de arquitetura

A suíte de arquitetura roda como um teste Go comum:

```bash
go test -count=1 ./architecture/...
```

Use sempre `-count=1` ao rodar (ou re-rodar) esta suíte manualmente: como
`TestRepoCompliance` carrega os pacotes reais do repositório em tempo de
execução (via `packages.Load`, não via imports estáticos do próprio pacote
`architecture`), o build cache do Go não sabe que o resultado depende do
conteúdo de `internal/...`. Sem `-count=1`, uma segunda execução pode
retornar `ok` (em cache) mesmo depois de você introduzir uma violação real.

Ela carrega o grafo de pacotes do módulo (via
`golang.org/x/tools/go/packages`) e aplica um conjunto de regras (R1–R6)
descritas em [`architecture/rules.go`](architecture/rules.go), cada uma
testada contra fixtures inválidas propositalmente em
`architecture/testdata/fixtures/`. Um meta-teste anti-vacuidade garante que
nenhuma regra "passa" só porque não encontrou nada para checar — toda regra
que não está marcada `AllowEmpty` precisa casar pelo menos uma violação nos
testes, ou o meta-teste falha a suíte. Um teste separado,
`TestRepoCompliance`, carrega o repositório real (não as fixtures) e exige
zero violações — é esse teste que a demonstração manual abaixo exercita.

### Passo a passo de verificação manual (import quebrando a regra R1)

Este passo a passo demonstra, na prática, que a suíte pega uma violação real
introduzida no código do repositório (não só nas fixtures de teste):

1. Baseline — confirme que a suíte passa limpa:

   ```bash
   go test -count=1 ./architecture/...
   # ok  	github.com/manuellysuzik/mesaposta-api/architecture
   ```

2. Crie um pacote de domínio fake importando `net/http` diretamente:

   ```bash
   mkdir -p internal/platform/tenant_test_demo/domain
   cat > internal/platform/tenant_test_demo/domain/demo.go <<'EOF'
   package domain

   import "net/http"

   func Ping() int { return http.StatusOK }
   EOF
   ```

3. Rode a suíte de novo — ela deve **falhar**, reportando a violação de R1
   (pacote sob `domain/**` alcançando `net/http`):

   ```bash
   go test -count=1 ./architecture/...
   # --- FAIL: TestRepoCompliance
   #     architecture fitness function found 1 violation(s) in the real repository:
   #     R1: .../internal/platform/tenant_test_demo/domain: reaches forbidden dependencies: net/http (via net/http), ...
   ```

4. Remova o import quebrado (ou o arquivo/pacote inteiro):

   ```bash
   rm -rf internal/platform/tenant_test_demo
   ```

5. Rode a suíte novamente e confirme que volta a passar limpa:

   ```bash
   go test -count=1 ./architecture/...
   # ok  	github.com/manuellysuzik/mesaposta-api/architecture
   ```

O pacote `tenant_test_demo` criado nos passos 2–3 é só para demonstração e
**não fica no repositório** — apenas as fixtures em
`architecture/testdata/fixtures/` permanecem versionadas.

## CI

`.github/workflows/ci.yml` roda em todo push/PR para `main`, com dois jobs
independentes — veja
[`docs/adr/0004-ci-pipeline-and-coverage-gate.md`](docs/adr/0004-ci-pipeline-and-coverage-gate.md)
para o racional completo:

- **check**: `gofmt`, `go vet`, `golangci-lint`, `go test -race ./...`
  (equivalente a `make check`), `gosec` e `govulncheck`. Não precisa de
  Docker.
- **integration**: `go test -tags=integration -race ./...` (todas as
  suítes contra Postgres real, incluindo a fatia end-to-end T19), mais o
  gate de cobertura combinada (unit + integration, ≥ 80%).

## Estrutura do repositório

```
.
├── architecture/            # fitness function de arquitetura
│   ├── rules.go
│   ├── arch_test.go
│   └── testdata/fixtures/   # pacotes fixture, cada um violando 1 regra
├── cmd/api/                 # composition root / entry point
│   ├── main.go               # os.Exit(run(...)) — sem lógica própria
│   └── run.go                 # config -> pool -> migrations opcionais ->
│                              # app.NewApp -> httpx.NewServer -> shutdown
├── docs/
│   ├── ARCHITECTURE.md       # visão geral + roteiro de leitura p/ a equipe
│   └── adr/                   # Architecture Decision Records
├── internal/
│   ├── modules/               # módulos de domínio
│   │   ├── README.md          # template de layout que todo módulo segue
│   │   ├── operations/        # Fase 1 — estoque, compras
│   │   ├── catalog/           # Fase 2 — produtos, receitas
│   │   ├── crm/               # Fase 4 — clientes, fidelidade
│   │   ├── community/         # Fase 5 — posts, curtidas
│   │   └── ordering/          # Fase 3 — mesas, pedidos + cardápio público
│   └── platform/               # infraestrutura compartilhada
│       ├── app/                 # NewApp: monta o http.Handler completo
│       ├── auth/                 # Authenticate/RequireRole + token/refresh/
│       │   │                      # password/publictenant (resolução por slug)
│       ├── config/                # carregamento/validação de configuração
│       ├── db/                     # Pool + Scope (RLS) + migrations +
│       │                            # registro de tenants (LookupTenantBySlug)
│       ├── errs/                    # taxonomia de erros
│       ├── httpx/                    # envelope, roteador, middlewares,
│       │                              # rate limiter, E2E
│       ├── outbox/                    # outbox transacional (+ outbox/postgres)
│       └── tenant/                     # TenantID + contexto
├── BACKLOG.md                 # melhorias identificadas e deliberadamente
│                                # adiadas, com justificativa
├── .github/workflows/ci.yml   # CI: check + integration
├── .editorconfig
├── .golangci.yml
├── .gitignore
├── Makefile
└── go.mod
```
