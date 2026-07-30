# Flexberry

CLI experimental para migrations, ORM e factories em projetos Go.

## Instalação

### Downloads

| Sistema | Arquitetura | Download |
|:---|:---|:---|
| Windows | x86-64 / AMD64 | [flexberry-windows-amd64.exe](https://github.com/PhelipeViana/flexberry/releases/download/v0.1.0-alpha.15/flexberry-windows-amd64.exe) |
| Linux | x86-64 / AMD64 | [flexberry-linux-amd64](https://github.com/PhelipeViana/flexberry/releases/download/v0.1.0-alpha.15/flexberry-linux-amd64) |
| Verificação | SHA-256 | [checksums.txt](https://github.com/PhelipeViana/flexberry/releases/download/v0.1.0-alpha.15/checksums.txt) |

Todos os arquivos e versões anteriores ficam disponíveis em
[GitHub Releases](https://github.com/PhelipeViana/flexberry/releases).

No Windows, renomeie o arquivo para `flexberry.exe`, coloque-o na raiz do
projeto, ao lado do `go.mod`, e execute:

```powershell
.\flexberry.exe
```

No Linux:

```bash
mv flexberry-linux-amd64 flexberry
chmod +x flexberry
./flexberry
```

O menu é carregado do arquivo público `config/menu.json`. Sem internet, o
executável usa um menu local seguro. O manifesto online pode apenas habilitar
comandos conhecidos; ele não executa scripts arbitrários.

Ao iniciar, o Flexberry também consulta os releases publicados. Quando encontra
uma versão mais nova, o menu normal é substituído pelas opções de baixar e
instalar automaticamente ou sair. No Windows, o download é validado com o
`checksums.txt`, o executável atual é preservado como `.old` e a versão nova é
aberta automaticamente.

## Comandos

```powershell
.\flexberry.exe config install
.\flexberry.exe config update
.\flexberry.exe config remove

.\flexberry.exe connection report

.\flexberry.exe migrate reload
.\flexberry.exe migrate run

.\flexberry.exe factory reload
.\flexberry.exe factory run

.\flexberry.exe orm reload
.\flexberry.exe orm run

.\flexberry.exe validate
.\flexberry.exe version
.\flexberry.exe self update
```

`connection report` abre e autentica em todas as conexões do
`connection.yaml`. O relatório mostra status, dialeto, schema, versão do banco,
tempo de resposta e destaca a conexão padrão.

`config install` cria somente configurações editáveis:

```text
internal/flexberry/
├── connection.yaml
├── migrate.yaml
├── orm.yaml
└── factory.yaml
```

- `connection.yaml`: ambientes e conexões.
- `migrate.yaml`: entidades monitoradas e histórico das migrations.
- `orm.yaml`: entidades de origem e destino do ORM.
- `factory.yaml`: destino, defaults e expressões das factories.

Os arquivos contêm comentários explicando cada campo.

## Fluxo

```powershell
.\flexberry.exe config install

# Revise os quatro arquivos YAML.

.\flexberry.exe migrate reload
.\flexberry.exe migrate run
.\flexberry.exe factory reload
.\flexberry.exe factory run
.\flexberry.exe orm reload
```

`migrate reload` compara as entidades com o snapshot monitorado e cria um plano
neutro, imutável e nomeado por timestamp. `migrate run` converte esse plano
para o dialeto da conexão padrão e registra o checksum em `migrations_flex`.
Para migrar outro banco, altere `DB_DIALECT` no `.env`.

O package Go do ORM e das factories é inferido automaticamente pelo último
diretório configurado em `path`; não é necessário informar `package`.

`factory reload` sincroniza o ORM antes de gerar as factories. Regras do
`factory.yaml` funcionam como interceptadores e podem usar `COLUNA` ou
`TABELA.COLUNA`. A prioridade é: vínculo ORM, `exact` específico, `exact`
global, `contains`, expressão existente e fallback pelo tipo Go.

O scaffold inclui regras semânticas editáveis para nomes como `NOME`, `EMAIL`,
`CPF`, `CNPJ`, `CEP`, `TELEFONE`, `HASH`, `DESCRICAO`, `CIDADE` e `ATIVO`.
Os limites ficam visíveis no YAML e podem ser adaptados ao schema:

```yaml
expressions:
  exact:
    ATIVO: flexberry.FakeIntRange(index, 0, 1)
  contains:
    - pattern: EMAIL
      expression: flexberry.FakeEmail(index, 150)
```

Os helpers limitam texto por caracteres Unicode, preservam o sufixo
determinístico quando possível e aceitam limites opcionais:

```go
flexberry.FakeName(index, 100)
flexberry.FakeCPF(index, 14)
flexberry.FakeText(index, 255)
```

Relacionamentos `belongsTo` geram:

```go
flexberry.Vinculo("tabela_pai", "coluna_pai")
```

`factory run` cria um runner Go temporário dentro do projeto, testa a conexão,
limpa tabelas filhas antes das pais e insere pais antes dos filhos. O runner é
removido ao final.

## Desenvolvimento

```powershell
go fmt ./...
go vet ./...
go test ./...
go build -o flexberry.exe ./cmd/flexberry
```

## Publicação

Os executáveis são publicados automaticamente pelo workflow
`.github/workflows/release.yml`. Para publicar uma nova versão, crie e envie
uma tag semântica:

```powershell
git tag v0.1.0-alpha.16
git push origin v0.1.0-alpha.16
```

O GitHub Actions executa os testes, injeta `0.1.0-alpha.16` no binário, compila
Windows e Linux, gera `checksums.txt` e publica o release. Tags com hífen, como
as versões `alpha`, são marcadas automaticamente como pré-release.
