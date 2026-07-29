# Flexberry

CLI experimental para geração de ORM e factories em projetos Go.

## Instalação

Baixe `flexberry-windows-amd64.exe` na página de Releases, renomeie para
`flexberry.exe` e coloque-o na raiz do projeto, ao lado do `go.mod`.

```powershell
.\flexberry.exe
```

O menu é carregado do arquivo público `config/menu.json`. Sem internet, o
executável usa um menu local seguro. O manifesto online pode apenas habilitar
comandos conhecidos; ele não executa scripts arbitrários.

## Comandos

```powershell
.\flexberry.exe config install
.\flexberry.exe config update
.\flexberry.exe config remove

.\flexberry.exe connection report
.\flexberry.exe orm reload
.\flexberry.exe orm run

.\flexberry.exe factory reload
.\flexberry.exe factory run

.\flexberry.exe validate
.\flexberry.exe version
```

`connection report` abre e autentica em todas as conexões do
`flexberry.yaml`. O relatório mostra status, dialeto, schema, versão do banco,
tempo de resposta e destaca a conexão padrão.

`config install` cria somente configurações editáveis:

```text
internal/flexberry/
├── flexberry.yaml
├── orm.yaml
└── factory.yaml
```

- `flexberry.yaml`: ambientes e conexões.
- `orm.yaml`: entidades de origem e destino do ORM.
- `factory.yaml`: destino, defaults e expressões das factories.

Os arquivos contêm comentários explicando cada campo.

## Fluxo

```powershell
.\flexberry.exe config install

# Revise os três arquivos YAML.

.\flexberry.exe orm reload
.\flexberry.exe factory reload
.\flexberry.exe factory run
```

O package Go do ORM e das factories é inferido automaticamente pelo último
diretório configurado em `path`; não é necessário informar `package`.

`factory reload` sincroniza o ORM antes de gerar as factories. Expressões
editadas nos arquivos existentes são preservadas. Regras declaradas no
`factory.yaml` podem usar `COLUNA` ou `TABELA.COLUNA`.

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
