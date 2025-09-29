# Sistema de Cadastro Unificado da Stone Co: Evolução Arquitetural de User/Organization para Legal Entity e Stone Account

## Resumo Executivo

Este documento apresenta uma análise dissertativa sobre a evolução do sistema de cadastro da Stone Co, abordando a transição do modelo antigo baseado em User/Organization para a nova arquitetura centralizada em Legal Entity, Stone Account e o sistema SALEM (Stone Account and Legal Entity Manager). A transformação representa um marco fundamental na unificação de dados cadastrais das marcas Stone, TON e Pagar.me, visando eliminar redundâncias, reduzir custos operacionais e possibilitar a oferta de múltiplos produtos por documento.

## 1. Introdução

A Stone Co enfrentou desafios significativos com seu modelo de cadastro descentralizado, onde cada marca (Stone, TON, Pagar.me) mantinha bases de dados independentes, resultando em duplicação de informações cadastrais, custos elevados de KYC (Know Your Customer) e análises de bureau, além da impossibilidade de ofertar múltiplos produtos para um mesmo cliente. Este cenário motivou o desenvolvimento de uma nova arquitetura de cadastro unificado, culminando na criação do sistema SALEM e na reformulação completa das entidades cadastrais.

## 2. Fundamentação Conceitual

### 2.1 Tenant

O conceito de Tenant representa uma abstração técnica fundamental para separar contextos de entidades dentro do ecossistema Stone Co. Conforme definido na documentação oficial, "Tenant é o termo técnico para abstrair/separar um contexto maior de entidades quaisquer" ¹.

Atualmente, o sistema reconhece três tenants principais:
- **TON**: Marca focada em micro e pequenos empreendedores
- **Stone**: Marca tradicional para médias e grandes empresas
- **Pagar.me**: Plataforma de pagamentos online

A arquitetura prevê que, após o processo de unificação, haverá convergência para um único tenant denominado "StoneCo", mantendo flexibilidade para criação de novos tenants conforme necessidades futuras do negócio.

### 2.2 Legal Entity (Entidade Legal)

A Legal Entity representa uma pessoa física ou jurídica na base de dados unificada, constituindo a fundação do novo modelo cadastral. Diferentemente do modelo anterior, garante-se unicidade por documento (CPF ou CNPJ), eliminando duplicações que caracterizavam o sistema legado ².

#### Características Técnicas:
- **Unicidade documental**: Uma única entidade por documento
- **Tipologia**: `natural_person` (pessoa física) ou `juridical_person` (pessoa jurídica)
- **Dados estruturados**: Nome legal, documento, validações de risco, dados de bureau
- **Status de confiança**: `trusted`/`untrusted` baseado em validações biométricas e de KYC

O modelo de Legal Entity resolve problemas fundamentais do sistema anterior, onde a mistura entre entidade cadastral e identidade de acesso (no caso da entidade User) criava inconsistências e limitações operacionais.

### 2.3 Stone Account

A Stone Account constitui um contexto cadastral que estabelece a relação entre Legal Entity e Identidade de Acesso, sendo definida como "A relação entre Entidade Legal e Identidade de Acesso, que definem o Cadastro do cliente na Stone Co" ³.

#### Funcionalidades Principais:
- **Contextualização**: Armazena dados autodeclarados específicos por contexto (ex: endereço de entrega para um produto específico)
- **Tenant Association**: Cada Stone Account possui um tenant que determina sua classificação lógica
- **Hierarquização**: Suporte a estruturas pai/filho através dos campos `path` e `parent_id`
- **Status de Confiança**: Proxy para relações `identity_ownership_claims` e `relation_claims`

É importante ressaltar que a Stone Account não possui relação com contas de pagamento do sistema Banking, conforme explicitado na documentação: "Não tem nenhuma relação com a conta de pagamento de banking" ³.

### 2.4 SALEM (Stone Account and Legal Entity Manager)

O SALEM representa o sistema central da Plataforma de Cadastro, sendo responsável por "armazenar e disponibilizar, de forma centralizada, dados cadastrais de Pessoas Físicas e Jurídicas que possuem relacionamento com a Stone" ⁴.

#### Arquitetura Técnica:
- **Linguagem**: Elixir com framework Phoenix
- **Banco de Dados**: PostgreSQL (compartilhado com RAD por questões de planejamento)
- **Message Broker**: Apache Kafka para Event Bus
- **Background Jobs**: Oban Jobs para processamento assíncrono
- **Telemetria**: Stack completa de observabilidade

#### Responsabilidades Sistêmicas:
1. **Gerenciamento de Entidades Legais**: Criação e manutenção de pessoas físicas e jurídicas
2. **Gerenciamento de Stone Accounts**: Administração de contextos cadastrais
3. **Consolidação de Dados**: Agregação e disponibilização via APIs
4. **Mecanismos de Confiança**: Implementação de relações baseadas em análises KYC

## 3. Evolução Arquitetural dos Modelos

### 3.1 Modelo Legado: User + Organization

O modelo original da Stone Co caracterizava-se pela dualidade conceitual da entidade User, que exercia tanto função de entidade cadastral quanto identidade de acesso. Este design apresentava limitações significativas:

#### Estrutura do Modelo Legado:
- **User**: Entidade com papel duplo (cadastro PF + identidade de acesso)
- **Organization**: Entidade cadastral exclusiva para pessoas jurídicas
- **Stone Code**: Identificador único da relação cliente-produto

#### Limitações Identificadas:
1. **Mistura conceitual**: User amalgamava responsabilidades cadastrais e de autenticação
2. **Duplicação de dados**: Múltiplas entidades para um mesmo documento
3. **Custos elevados**: KYC e consultas de bureau repetidas
4. **Restrição de produtos**: Impossibilidade de múltiplas contas por documento

### 3.2 Modelo Intermediário: Legal Entity

A introdução do conceito de Legal Entity representou um passo evolutivo importante, separando claramente entidades cadastrais de identidades de acesso:

#### Estrutura do Modelo Intermediário:
- **User**: Exclusivamente identidade de acesso
- **Legal Entity**: Entidade cadastral única por documento
- **Manutenção**: Stone Codes preservados para compatibilidade

Este modelo foi implementado inicialmente para suportar o tombamento das contas Pagar.me e posteriormente adotado pelas novas contas TON criadas após a migração para BaaS v4.

### 3.3 Modelo Atual: Legal Entity + Stone Account + SALEM

A arquitetura atual representa a consolidação da visão de cadastro unificado, introduzindo a Stone Account como camada intermediária entre Legal Entity e produtos:

#### Componentes da Arquitetura Atual:
- **LEA (Legal Entity Affiliation)**: Orquestrador de credenciamento unificado
- **Legal Entity**: Dados de bureau únicos por documento
- **Stone Account**: Contextos cadastrais com dados autodeclarados
- **User Identity**: Identidades de acesso vinculadas a Stone Accounts
- **Tenant**: Classificação por marca
- **SALEM**: Sistema gerenciador centralizado
- **QUAD**: Sistema de questionários dinâmicos

#### Fluxo Arquitetural Completo:

O modelo atual estabelece uma arquitetura orquestrada onde cada componente possui responsabilidades específicas:

```
Sistemas de Distribuição (Apps/Web)
          ↓
    LEA (Orquestrador)
          ↓
   Análise de Risco ← → QUAD (Questionários)
          ↓
  SALEM (Gerenciador)
          ↓
Legal Entity ← → Stone Account ← → User Identity
          ↓
    Produtos + Stone Codes
```

Esta arquitetura moderna resolve as limitações históricas através de:
- **Separação de responsabilidades**: Cada sistema com função específica
- **Orquestração centralizada**: LEA coordena toda jornada de credenciamento
- **Reutilização inteligente**: Legal Entity compartilhada, contextos específicos via Stone Account
- **Flexibilidade multi-tenant**: Suporte nativo a múltiplas marcas

## 4. Stone Codes: Compatibilidade e Integração

Os Stone Codes representam "uma relação única entre um cliente e um produto/oferta" ⁵, sendo mantidos no novo modelo para garantir compatibilidade com sistemas de adquirência existentes. Apesar da reformulação arquitetural, os Stone Codes preservam sua função original de identificação cliente-produto, adaptando-se às novas estruturas de dados.

### 4.1 Integração com Novo Modelo

No modelo atual, os Stone Codes mantêm associação com Stone Accounts ao invés de diretamente com Legal Entities, permitindo:
- Granularidade por contexto (tenant)
- Múltiplos códigos por documento
- Compatibilidade com sistemas legados
- Transição suave para adquirência

### 4.5 LEA (Legal Entity Affiliation): Orquestrador de Credenciamento

O LEA (Legal Entity Affiliation) representa um componente fundamental na nova arquitetura de credenciamento da Stone Co, atuando como orquestrador centralizado que coordena a integração entre sistemas de Distribuição, SALEM e plataformas de análise de risco.

#### Características do LEA:
- **Coordenação de Fluxos**: Gerencia a jornada completa de credenciamento desde a solicitação inicial até a criação de Legal Entity e Stone Account
- **Integração Sistêmica**: Atua como ponto de integração entre:
  - Sistemas de Distribuição (Apps/Web)
  - SALEM (Stone Account and Legal Entity Manager)
  - Sistemas de análise de risco e compliance
  - QUAD (Questions and Answers Dispatcher) para questionários dinâmicos

#### Responsabilidades Técnicas:
1. **Gestão de Affiliation Requests**: Processamento de solicitações via `/v1/affiliation-requests`
2. **Orquestração de KYC**: Coordenação com QUAD para questionários personalizados
3. **Análise de Risco**: Integração com sistemas de validação biométrica e documental
4. **Criação de Entidades**: Coordenação com SALEM para criação de Legal Entities e Stone Accounts

#### Substituição do RAD:
O LEA representa a evolução natural do RAD (Resource Affiliation Dispatcher), oferecendo:
- Unificação de fluxos de credenciamento entre marcas
- Arquitetura moderna preparada para escala
- Integração nativa com o modelo Legal Entity/Stone Account
- Suporte a credenciamento multi-tenant

A introdução do LEA elimina a necessidade de múltiplos pontos de entrada para credenciamento, consolidando a experiência do cliente e reduzindo a complexidade operacional.

## 5. Processo de Migração e Estados Atuais

### 5.1 Estratégia de Migração

A Stone Co adotou uma estratégia de migração gradual, mantendo convivência entre modelos antigo e novo durante o período de transição:

#### Status por Marca:
- **Pagar.me**: Migração completa para Legal Entity
- **TON**: Novas contas via Legal Entity (pós-BaaS v4), tombamento de contas antigas em andamento
- **Stone**: Dual mode - convivência entre User/Organization e Legal Entity

### 5.2 Sistemas Envolvidos na Migração

#### Modelo Antigo:
- **PandA**: Gerenciamento de User/Organization via Sign-Up
- **Fluxo direto**: Cliente → User/Organization → Produtos → Stone Codes

#### Modelo Novo:
- **LEA (Legal Entity Affiliation)**: Coordenação de credenciamentos
- **SALEM**: Gerenciamento centralizado de entidades
- **QUAD**: Transformação de pendências em questionários
- **Fluxo estruturado**: Cliente → LEA → SALEM → Legal Entity/Stone Account → Produtos

#### Detalhamento do Fluxo LEA → SALEM:

O fluxo moderno de credenciamento estabelece uma orquestração sofisticada entre os componentes:

1. **Iniciação via LEA**:
   - Cliente solicita credenciamento através de sistemas de Distribuição
   - LEA recebe `/v1/affiliation-requests` com dados iniciais
   - Validações preliminares e categorização por tenant

2. **Análise e Enriquecimento**:
   - LEA coordena com sistemas de análise de risco
   - Integração com QUAD para questionários dinâmicos via `/v1/response-forms`
   - Validações biométricas e documentais quando necessário

3. **Criação de Entidades via SALEM**:
   - LEA instrui SALEM para criação de Legal Entity (dados de bureau)
   - Criação de Stone Account associada (dados autodeclarados + contexto)
   - Estabelecimento de relações de confiança (`trusted`/`untrusted`)

4. **Integração com Produtos**:
   - Stone Account vinculada a produtos específicos
   - Geração de Stone Codes para sistemas de adquirência
   - Notificação aos sistemas downstream

Este fluxo substitui o modelo anterior onde PandA gerenciava diretamente User/Organization, introduzindo camadas de orquestração que permitem:
- **Reutilização de KYC**: Dados de bureau centralizados na Legal Entity
- **Flexibilidade de contexto**: Multiple Stone Accounts por Legal Entity
- **Análise de risco integrada**: Decisões informadas antes da criação de entidades

### 5.3 Desafios da Transição

O período de convivência entre modelos apresenta desafios operacionais significativos:

1. **Dualidade de APIs**: Manutenção simultânea de endpoints antigos e novos
2. **Complexidade de autorização**: Suporte a diferentes tipos de entidades em sistemas de borda
3. **Consistência de dados**: Garantia de integridade durante a migração
4. **Treinamento de equipes**: Adaptação a novos conceitos e fluxos

## 6. Consolidação de Dados e APIs

### 6.1 Tipos de Consolidação

O SALEM implementa três modalidades de consolidação de dados:

#### Consolidação por Legal Entity:
- **Critério**: `legal_entity_id = X AND stone_account_id IS NULL AND identity_id IS NULL`
- **Escopo**: Todos os dados relacionados à entidade legal
- **Filtros**: Exclusão de dados `untrusted` para prevenção de fraudes

#### Consolidação por Stone Account:
- **Critério**: `legal_entity_id = X AND stone_account_id = Y AND identity_id IS NULL`
- **Escopo**: Dados específicos do contexto cadastral
- **Exclusões**: Dados de bureau (vinculados à Legal Entity) e outras Stone Accounts

#### Consolidação por Identidade de Acesso:
- **Status**: Existente, mas uso não recomendado
- **Razão**: Cada identidade possui Stone Account específica

### 6.2 Mapeamento de APIs

A transição entre modelos requer adaptação significativa nos endpoints de API:

| Operação | API Legada | API Atual |
|----------|------------|-----------|
| Criação de conta | `/users/signup` | `/v1/affiliation-requests` |
| Dados PF | `/users` | `/legal-entities` + `/affiliation-requests/{id}` |
| Dados PJ | `/organizations` | `/legal-entities` + `/affiliation-requests/{id}` |
| KYC PF | `/users/{id}/kyc/answer` | `/v1/response-forms` |
| KYC PJ | `/organizations/{id}/kyc/answer` | `/v1/response-forms` |

### 6.3 APIs de Credenciamento Unificado

A introdução do LEA como orquestrador trouxe uma nova camada de APIs especificamente desenhadas para o credenciamento unificado, estabelecendo padrões consistentes entre marcas e produtos.

#### Endpoints Principais do LEA:

**Criação de Solicitação de Afiliação:**
```
POST /v1/affiliation-requests
```
- **Função**: Iniciar processo de credenciamento unificado
- **Payload**: Dados básicos do cliente + contexto (tenant, produto)
- **Resposta**: `affiliation_request_id` para tracking do fluxo
- **Integração**: Substitui múltiplos endpoints de signup por marca

**Consulta de Estado da Afiliação:**
```
GET /v1/affiliation-requests/{id}
```
- **Função**: Monitoring do progresso de credenciamento
- **Estados**: `pending`, `under_analysis`, `approved`, `rejected`, `completed`
- **Resposta**: Status atual + próximos passos necessários

**Submissão de Questionários KYC:**
```
POST /v1/response-forms
```
- **Função**: Resposta a questionários dinâmicos via QUAD
- **Integração**: Coordenado pelo LEA baseado no perfil de risco
- **Flexibilidade**: Questionários adaptativos por tenant/produto

#### Fluxo de Estados de uma Affiliation Request:

1. **`pending`**: Solicitação criada, aguardando análise inicial
2. **`under_analysis`**: Em processamento, potenciais chamadas para KYC/documentação
3. **`approved`**: Validações completas, criação de Legal Entity/Stone Account autorizada
4. **`completed`**: Entidades criadas no SALEM, cliente pronto para produtos
5. **`rejected`**: Não aprovado em validações de risco/compliance

#### Benefícios da API Unificada:

- **Consistência**: Mesmo contrato para todas as marcas
- **Tracking centralizado**: Visibilidade completa do pipeline de credenciamento
- **Flexibilidade**: Questionários e validações adaptáveis
- **Observabilidade**: Métricas unificadas para toda jornada
- **Manutenibilidade**: Single source of truth para credenciamento

#### Migração de APIs Legadas:

| Operação | API Legada (por marca) | API Unificada (LEA) |
|----------|------------------------|---------------------|
| Início de cadastro | `/users/signup`, `/organizations/signup` | `/v1/affiliation-requests` |
| Status do processo | Múltiplos endpoints por marca | `/v1/affiliation-requests/{id}` |
| KYC/Questionários | APIs específicas por contexto | `/v1/response-forms` |
| Consulta de dados | Endpoints por entidade | Consolidado via SALEM APIs |

## 7. Benefícios e Impactos da Nova Arquitetura

### 7.1 Benefícios para o Cliente

A nova arquitetura proporciona experiência aprimorada através de:
- **Múltiplas contas por documento**: Superação da limitação histórica
- **Experiência unificada**: Consistência entre marcas Stone, TON e Pagar.me
- **Dados consistentes**: Eliminação de discrepâncias entre plataformas
- **Agilidade em KYC**: Reutilização de validações já realizadas

### 7.2 Benefícios Operacionais

Para a Stone Co, a unificação resulta em:
- **Redução de custos**: KYC e consultas de bureau executadas uma única vez por documento
- **Visão 360° do cliente**: Consolidação de todos os relacionamentos
- **Flexibilidade produto**: Facilidade para criação de novas ofertas
- **Compliance aprimorado**: Controle centralizado de validações regulatórias

### 7.3 Benefícios Técnicos

A arquitetura moderna oferece:
- **Escalabilidade**: Design preparado para crescimento
- **Observabilidade**: Telemetria completa com Oban Jobs e Kafka
- **Flexibilidade**: Estrutura de tenants adaptável
- **Integridade**: Mecanismos de confiança baseados em validações rigorosas

## 8. Considerações Futuras e Roadmap

### 8.1 Próximas Etapas

A evolução contínua do sistema prevê:
- **Tombamento completo**: Migração de todas as contas Stone legadas
- **Unificação de tenant**: Convergência para StoneCo único
- **Descontinuação gradual**: Sunset do modelo User/Organization
- **Otimização de performance**: Melhorias contínuas no SALEM

### 8.2 Desafios Futuros

Os principais desafios incluem:
- **Governança de dados**: Manutenção da qualidade em escala
- **Performance**: Otimização de consultas com volume crescente
- **Regulatory compliance**: Adaptação a novas exigências regulatórias
- **Integration complexity**: Gestão de integrações com sistemas terceiros

### 8.3 Estratégia de Credenciamento Unificado

A consolidação completa da arquitetura de credenciamento representa um dos pilares estratégicos mais importantes para o futuro da Stone Co, estabelecendo uma base sólida para expansão e inovação contínua.

#### Visão de Futuro:

**Credenciamento Zero-Touch:**
- Automação completa baseada em análise de risco integrada
- Decisões instantâneas para perfis de baixo risco
- Intervenção humana apenas para casos específicos

**Unificação Completa de Tenants:**
- Convergência para tenant único "StoneCo"
- Experiência consistente independente do ponto de entrada
- Portabilidade completa de contextos entre produtos

**Inteligência Preditiva:**
- Análise comportamental para prevenção de fraudes
- Scoring dinâmico baseado em múltiplas fontes de dados
- Personalização de jornadas por perfil de cliente

#### Roadmap de Consolidação:

**Fase 1 - Migração Completa (2025)**:
- Tombamento de todas as contas Stone legadas
- Descontinuação gradual do modelo User/Organization
- Unificação de APIs de credenciamento via LEA

**Fase 2 - Otimização (2025-2026)**:
- Convergência de tenants para StoneCo único
- Implementação de credenciamento zero-touch
- Integração com novas fontes de dados para análise de risco

**Fase 3 - Inovação (2026+)**:
- Credenciamento preditivo baseado em ML
- Expansão para novos mercados e produtos
- Plataforma como serviço para terceiros

#### Benefícios Estratégicos da Unificação:

**Para a Stone Co:**
- **Redução de custos**: Eliminação de redundâncias sistêmicas
- **Time to market**: Agilidade para lançamento de novos produtos
- **Visão 360°**: Conhecimento completo do cliente
- **Compliance centralizado**: Controle unificado de regulamentações

**Para Clientes:**
- **Experiência fluida**: Jornada consistente entre marcas
- **Múltiplos produtos**: Acesso facilitado a todo ecossistema Stone
- **Onboarding ágil**: Reutilização de validações já realizadas
- **Suporte unificado**: Atendimento integrado cross-produto

#### Métricas de Sucesso:

- **Time to Live**: Redução do tempo de credenciamento em 70%
- **Custo por credenciamento**: Diminuição de 50% via reutilização de KYC
- **Taxa de aprovação**: Melhoria através de análises mais precisas
- **Satisfação do cliente**: NPS unificado superior a 70

A estratégia de credenciamento unificado posiciona a Stone Co para dominar o mercado de serviços financeiros através de eficiência operacional, experiência superior do cliente e capacidade de inovação acelerada.

## 9. Conclusão

A evolução do sistema de cadastro da Stone Co de User/Organization para Legal Entity e Stone Account, orquestrada pelo SALEM, representa uma transformação arquitetural fundamental que posiciona a empresa para crescimento sustentável e inovação contínua. A nova arquitetura não apenas resolve limitações históricas do modelo legado, mas estabelece fundações sólidas para expansão de produtos, melhoria da experiência do cliente e otimização operacional.

A estratégia de migração gradual demonstra maturidade técnica e organizacional, permitindo transição suave sem interrupção de serviços críticos. O sucesso desta iniciativa consolidará a Stone Co como referência em gestão unificada de dados cadastrais no setor de serviços financeiros brasileiro.

## Referências

¹ Stone Co. "GLO0009 - Tenant". *Confluence - Produtos de Identidade*. Disponível em: https://allstone.atlassian.net/wiki/spaces/PDID/pages/7815233582

² Stone Co. "Legal Entities". *Confluence - Squad Cadastro & Dados (CADA)*. Disponível em: https://allstone.atlassian.net/wiki/spaces/STAA/pages/5991825453

³ Stone Co. "GLO0007 - Stone Account". *Confluence - Produtos de Identidade*. Disponível em: https://allstone.atlassian.net/wiki/spaces/PDID/pages/7552303933

⁴ Stone Co. "[PCAD] Arquitetura SALEM". *Confluence - Produtos de Identidade*. Disponível em: https://allstone.atlassian.net/wiki/spaces/PDID/pages/8182662646

⁵ Stone Co. "GLO0008 - Stone Code". *Confluence - Produtos de Identidade*. Disponível em: https://allstone.atlassian.net/wiki/spaces/PDID/pages/7552303951

⁶ Stone Co. "Banking: Adequação ao novo modelo de cadastro e oferta". *Confluence - Banking*. Disponível em: https://allstone.atlassian.net/wiki/spaces/BANKING/pages/8252654433

⁷ Stone Co. "Mudanças relacionadas a mudança de user/org para entidades legais". *Confluence - Squad Cadastro & Dados (CADA)*. Disponível em: https://allstone.atlassian.net/wiki/spaces/STAA/pages/6264292016

⁸ Stone Co. "[PCAD] API de Leitura de Dados Cadastrais". *Confluence - Produtos de Identidade*. Disponível em: https://allstone.atlassian.net/wiki/spaces/PDID/pages/8082884162

⁹ Stone Co. "[PCAD] API de Listagem de Stone Accounts". *Confluence - Produtos de Identidade*. Disponível em: https://allstone.atlassian.net/wiki/spaces/PDID/pages/8095072268

¹⁰ Stone Co. "Integração para consultar dados cadastrais via API". *Confluence - Produtos de Identidade*. Disponível em: https://allstone.atlassian.net/wiki/spaces/PDID/pages/7964819626

¹¹ Stone Co. "[PCAD] Legal Entity Affiliation". *Confluence - Produtos de Identidade*. Disponível em: https://allstone.atlassian.net/wiki/spaces/PDID/pages/8163297155

¹² Stone Co. "[CADA] Estratégia de credenciamento unificado". *Confluence - Squad Cadastro & Dados (CADA)*. Disponível em: https://allstone.atlassian.net/wiki/spaces/STAA/pages/8231812459

¹³ Stone Co. "[PCAD] Pre-affiliation API". *Confluence - Produtos de Identidade*. Disponível em: https://allstone.atlassian.net/wiki/spaces/PDID/pages/8100970536

¹⁴ Stone Co. "Questions And Answers Dispatcher (QUAD)". *Confluence - Produtos de Identidade*. Disponível em: https://allstone.atlassian.net/wiki/spaces/PDID/pages/8134164020

¹⁵ Stone Co. "Arquitetura de Credenciamento Unificado - Visão Geral". *Confluence - Produtos de Identidade*. Disponível em: https://allstone.atlassian.net/wiki/spaces/PDID/pages/8219066850

---