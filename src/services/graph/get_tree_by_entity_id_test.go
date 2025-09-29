package graph_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"userprofilepoc/src/domain"
	"userprofilepoc/src/domain/entities"
	"userprofilepoc/src/helper/env"
	"userprofilepoc/src/infra/postgres"
	"userprofilepoc/src/repositories"
	"userprofilepoc/src/services/graph"
	"userprofilepoc/src/test_artefacts/comparer"
	"userprofilepoc/src/test_artefacts/stubs"
	"userprofilepoc/src/test_artefacts/test_seeder"
)

var _ = Describe("GetTreeByEntityID", func() {
	var (
		readWriteClient       *postgres.ReadWriteClient
		seeder                test_seeder.TestSeeder
		graphService          *graph.GraphService
		cachedGraphRepository *repositories.CachedGraphRepository
		graphQueryRepository  *repositories.GraphQueryRepository
		graphWriteRepository  *repositories.GraphWriteRepository
		ctx                   context.Context
		err                   error
	)

	dbReadHost := env.MustGetString("TEST_DB_READ_HOST")
	dbWriteHost := env.MustGetString("TEST_DB_WRITE_HOST")
	dbReadPort := env.GetString("TEST_DB_READ_PORT", "5432")
	dbWritePort := env.GetString("TEST_DB_WRITE_PORT", "5432")
	dbname := env.MustGetString("TEST_DB_NAME")
	dbUser := env.MustGetString("TEST_DB_USER")
	dbPassword := env.MustGetString("TEST_DB_PASSWORD")
	maxConnections := env.GetInt("TEST_DB_MAX_POOL_CONNECTIONS", 25)

	defaultDepth := 3
	defaultReferenceDate := time.Now()
	currentMonth := time.Now()
	lastMonth := time.Now().AddDate(0, -1, 0)    // Mês passado
	twoMonthsAgo := time.Now().AddDate(0, -2, 0) // Dois meses atrás

	BeforeEach(func() {
		ctx = context.Background()

		// Conexão com o banco de teste
		readWriteClient, err = postgres.NewReadWriteClient(dbReadHost, dbWriteHost, dbReadPort, dbWritePort, dbname, dbUser, dbPassword, maxConnections)
		if err != nil {
			panic(err)
		}

		// Setup dos componentes
		graphQueryRepository = repositories.NewGraphQueryRepository(readWriteClient.GetReadPool())
		graphWriteRepository = repositories.NewGraphWriteRepository(readWriteClient.GetWritePool(), nil)
		cachedGraphRepository = repositories.NewCachedGraphRepository(graphQueryRepository, nil)
		graphService = graph.NewGraphService(cachedGraphRepository, graphWriteRepository)
		seeder = test_seeder.New(readWriteClient.GetWritePool())

		// Limpar dados
		seeder.TruncateTables(ctx)
	})

	AfterEach(func() {
		if readWriteClient.GetReadPool() != nil {
			readWriteClient.GetReadPool().Close()
		}

		if readWriteClient.GetWritePool() != nil {
			readWriteClient.GetWritePool().Close()
		}
	})

	Context("Basic search", func() {
		Context("no entity exists in database", func() {
			When("searching for any id", func() {
				It("should return domain not found error", func() {
					// ARRANGE

					// ACT
					result, err := graphService.GetTreeByEntityID(ctx, 999, defaultDepth, defaultReferenceDate)

					// ASSERT
					Expect(err).To(HaveOccurred())
					Expect(result).To(BeNil())
					Expect(err).To(MatchError(domain.ErrEntityNotFound))
				})
			})
		})

		Context("only one entity exists in database", func() {
			When("searching for id that does not exist", func() {
				It("should return domain not found error", func() {
					// ARRANGE
					existingEntity := stubs.NewEntityStub().Get()
					seeder.InsertEntity(ctx, &existingEntity)

					// ACT
					result, err := graphService.GetTreeByEntityID(ctx, 999, defaultDepth, defaultReferenceDate)

					// ASSERT
					Expect(err).To(HaveOccurred())
					Expect(result).To(BeNil())
					Expect(err).To(MatchError(domain.ErrEntityNotFound))
				})
			})

			When("searching for existing id", func() {
				It("should return expected domain.NodeTree", func() {
					// ARRANGE
					existingEntity := stubs.NewEntityStub().Get()
					seeder.InsertEntity(ctx, &existingEntity)

					expected := &domain.NodeTree{
						Entity:       existingEntity,
						TemporalData: nil,
						Edges:        []*domain.ProfileEdge{},
					}

					// ACT
					result, err := graphService.GetTreeByEntityID(ctx, existingEntity.ID, defaultDepth, defaultReferenceDate)

					// ASSERT
					Expect(err).NotTo(HaveOccurred())
					Expect(result).To(BeComparableTo(
						expected,
						comparer.TimeWithinTolerance(200),
						comparer.JSONRawMessage(),
					))
				})
			})
		})

		Context("multiple entities exist in database", func() {
			When("searching for id that does not exist", func() {
				It("should return domain not found error", func() {
					// ARRANGE
					entity1 := stubs.NewEntityStub().Get()
					entity2 := stubs.NewEntityStub().Get()
					entity3 := stubs.NewEntityStub().Get()

					seeder.InsertEntity(ctx, &entity1)
					seeder.InsertEntity(ctx, &entity2)
					seeder.InsertEntity(ctx, &entity3)

					// ACT
					result, err := graphService.GetTreeByEntityID(ctx, 999, defaultDepth, defaultReferenceDate)

					// ASSERT
					Expect(err).To(HaveOccurred())
					Expect(result).To(BeNil())
					Expect(err).To(MatchError(domain.ErrEntityNotFound))
				})
			})

			When("searching for existing id", func() {
				It("should return expected domain.NodeTree", func() {
					// ARRANGE
					entity1 := stubs.NewEntityStub().Get()
					entity2 := stubs.NewEntityStub().Get()
					entity3 := stubs.NewEntityStub().Get()

					seeder.InsertEntity(ctx, &entity1)
					seeder.InsertEntity(ctx, &entity2)
					seeder.InsertEntity(ctx, &entity3)

					expected := &domain.NodeTree{
						Entity:       entity1,
						TemporalData: nil,
						Edges:        []*domain.ProfileEdge{},
					}

					// ACT
					result, err := graphService.GetTreeByEntityID(ctx, entity1.ID, defaultDepth, defaultReferenceDate)

					// ASSERT
					Expect(err).NotTo(HaveOccurred())
					Expect(result).To(BeComparableTo(
						expected,
						comparer.TimeWithinTolerance(200),
						comparer.JSONRawMessage(),
					))
				})
			})
		})
	})

	Context("Search with depth", func() {
		Context("entity with multiple levels", func() {
			When("searching with lower number of levels", func() {
				It("should return only entities from requested level in edges", func() {
					// ARRANGE
					rootEntity := stubs.NewEntityStub().Get()
					childEntity1 := stubs.NewEntityStub().Get()
					childEntity2 := stubs.NewEntityStub().Get()
					grandchildEntity := stubs.NewEntityStub().Get()

					seeder.InsertEntity(ctx, &rootEntity)
					seeder.InsertEntity(ctx, &childEntity1)
					seeder.InsertEntity(ctx, &childEntity2)
					seeder.InsertEntity(ctx, &grandchildEntity)

					edge1 := stubs.NewEdgeStub().WithLeftEntityID(rootEntity.ID).WithRightEntityID(childEntity1.ID).Get()
					edge2 := stubs.NewEdgeStub().WithLeftEntityID(rootEntity.ID).WithRightEntityID(childEntity2.ID).Get()
					edge3 := stubs.NewEdgeStub().WithLeftEntityID(childEntity1.ID).WithRightEntityID(grandchildEntity.ID).Get()

					seeder.InsertEdge(ctx, &edge1)
					seeder.InsertEdge(ctx, &edge2)
					seeder.InsertEdge(ctx, &edge3)

					expectedEdges := []*domain.ProfileEdge{
						&domain.ProfileEdge{
							Type: edge1.RelationshipType,
							Entity: &domain.NodeTree{
								Entity:       childEntity1,
								TemporalData: nil,
								Edges:        []*domain.ProfileEdge{},
							},
						},
						&domain.ProfileEdge{
							Type: edge2.RelationshipType,
							Entity: &domain.NodeTree{
								Entity:       childEntity2,
								TemporalData: nil,
								Edges:        []*domain.ProfileEdge{},
							},
						},
					}

					// ACT
					result, err := graphService.GetTreeByEntityID(ctx, rootEntity.ID, 1, defaultReferenceDate)

					// ASSERT
					Expect(err).NotTo(HaveOccurred())
					Expect(result.Edges).To(HaveLen(2)) // Apenas childEntity1 e childEntity2

					Expect(result.Edges).To(ContainElements(
						BeComparableTo(expectedEdges[0], comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
						BeComparableTo(expectedEdges[1], comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
					))
				})
			})

			When("searching with equal number of levels", func() {
				It("should return all associated entities in edges", func() {
					// ARRANGE
					rootEntity := stubs.NewEntityStub().Get()
					childEntity1 := stubs.NewEntityStub().Get()
					childEntity2 := stubs.NewEntityStub().Get()
					grandchildEntity := stubs.NewEntityStub().Get()

					seeder.InsertEntity(ctx, &rootEntity)
					seeder.InsertEntity(ctx, &childEntity1)
					seeder.InsertEntity(ctx, &childEntity2)
					seeder.InsertEntity(ctx, &grandchildEntity)

					edge1 := stubs.NewEdgeStub().WithLeftEntityID(rootEntity.ID).WithRightEntityID(childEntity1.ID).Get()
					edge2 := stubs.NewEdgeStub().WithLeftEntityID(rootEntity.ID).WithRightEntityID(childEntity2.ID).Get()
					edge3 := stubs.NewEdgeStub().WithLeftEntityID(childEntity1.ID).WithRightEntityID(grandchildEntity.ID).Get()

					seeder.InsertEdge(ctx, &edge1)
					seeder.InsertEdge(ctx, &edge2)
					seeder.InsertEdge(ctx, &edge3)

					expectedEdges := []*domain.ProfileEdge{
						&domain.ProfileEdge{
							Type: edge1.RelationshipType,
							Entity: &domain.NodeTree{
								Entity:       childEntity1,
								TemporalData: nil,
								Edges: []*domain.ProfileEdge{
									&domain.ProfileEdge{
										Type: edge3.RelationshipType,
										Entity: &domain.NodeTree{
											Entity:       grandchildEntity,
											TemporalData: nil,
											Edges:        []*domain.ProfileEdge{},
										},
									},
								},
							},
						},
						&domain.ProfileEdge{
							Type: edge2.RelationshipType,
							Entity: &domain.NodeTree{
								Entity:       childEntity2,
								TemporalData: nil,
								Edges:        []*domain.ProfileEdge{},
							},
						},
					}

					// ACT
					result, err := graphService.GetTreeByEntityID(ctx, rootEntity.ID, 2, defaultReferenceDate)

					// ASSERT
					Expect(err).NotTo(HaveOccurred())
					Expect(result.Edges).To(HaveLen(2)) // childEntity1 e childEntity2
					Expect(result.Edges).To(ContainElements(
						BeComparableTo(expectedEdges[0], comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
						BeComparableTo(expectedEdges[1], comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
					))
				})
			})

			When("searching with higher number of levels", func() {
				It("should return only associated entities in edges", func() {
					// ARRANGE
					rootEntity := stubs.NewEntityStub().Get()
					childEntity1 := stubs.NewEntityStub().Get()
					childEntity2 := stubs.NewEntityStub().Get()
					grandchildEntity := stubs.NewEntityStub().Get()

					seeder.InsertEntity(ctx, &rootEntity)
					seeder.InsertEntity(ctx, &childEntity1)
					seeder.InsertEntity(ctx, &childEntity2)
					seeder.InsertEntity(ctx, &grandchildEntity)

					edge1 := stubs.NewEdgeStub().WithLeftEntityID(rootEntity.ID).WithRightEntityID(childEntity1.ID).Get()
					edge2 := stubs.NewEdgeStub().WithLeftEntityID(rootEntity.ID).WithRightEntityID(childEntity2.ID).Get()
					edge3 := stubs.NewEdgeStub().WithLeftEntityID(childEntity1.ID).WithRightEntityID(grandchildEntity.ID).Get()

					seeder.InsertEdge(ctx, &edge1)
					seeder.InsertEdge(ctx, &edge2)
					seeder.InsertEdge(ctx, &edge3)

					expectedEdges := []*domain.ProfileEdge{
						&domain.ProfileEdge{
							Type: edge1.RelationshipType,
							Entity: &domain.NodeTree{
								Entity:       childEntity1,
								TemporalData: nil,
								Edges: []*domain.ProfileEdge{
									&domain.ProfileEdge{
										Type: edge3.RelationshipType,
										Entity: &domain.NodeTree{
											Entity:       grandchildEntity,
											TemporalData: nil,
											Edges:        []*domain.ProfileEdge{},
										},
									},
								},
							},
						},
						&domain.ProfileEdge{
							Type: edge2.RelationshipType,
							Entity: &domain.NodeTree{
								Entity:       childEntity2,
								TemporalData: nil,
								Edges:        []*domain.ProfileEdge{},
							},
						},
					}

					// ACT
					result, err := graphService.GetTreeByEntityID(ctx, rootEntity.ID, 5, defaultReferenceDate)

					// ASSERT
					Expect(err).NotTo(HaveOccurred())
					Expect(result.Edges).To(HaveLen(2)) // childEntity1 e childEntity2
					Expect(result.Edges).To(ContainElements(
						BeComparableTo(expectedEdges[0], comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
						BeComparableTo(expectedEdges[1], comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
					))
				})
			})
		})

		Context("entity has no levels", func() {
			When("searching with any depth", func() {
				It("should return empty list in edges", func() {
					// ARRANGE
					entity := stubs.NewEntityStub().Get()
					seeder.InsertEntity(ctx, &entity)

					// ACT
					result, err := graphService.GetTreeByEntityID(ctx, entity.ID, defaultDepth, defaultReferenceDate)

					// ASSERT
					Expect(err).NotTo(HaveOccurred())
					Expect(result.Entity.ID).To(Equal(entity.ID))
					Expect(result.Edges).To(BeEmpty())
				})
			})
		})
	})

	Context("Search with temporal data", func() {
		Context("entity with multiple temporal data for last month", func() {
			When("requesting temporal data from current month", func() {
				It("should return empty list in temporal data", func() {
					// ARRANGE
					entity := stubs.NewEntityStub().Get()
					seeder.InsertEntity(ctx, &entity)

					// Criar dados temporais para o mês passado
					temporal1 := stubs.NewTemporalPropertyStub().
						WithEntityID(entity.ID).
						WithKey("balance").
						WithReferenceDate(lastMonth).
						Get()

					temporal2 := stubs.NewTemporalPropertyStub().
						WithEntityID(entity.ID).
						WithKey("transactions").
						WithReferenceDate(lastMonth).
						Get()

					seeder.InsertTemporalProperty(ctx, temporal1)
					seeder.InsertTemporalProperty(ctx, temporal2)

					// ACT
					result, err := graphService.GetTreeByEntityID(ctx, entity.ID, defaultDepth, currentMonth)

					// ASSERT
					Expect(err).NotTo(HaveOccurred())
					Expect(result.TemporalData).To(BeEmpty())
				})
			})

			When("requesting temporal data from last month", func() {
				It("should return temporal data", func() {
					// ARRANGE
					entity := stubs.NewEntityStub().Get()
					seeder.InsertEntity(ctx, &entity)

					// Criar dados temporais para o mês passado
					temporal1 := stubs.NewTemporalPropertyStub().
						WithEntityID(entity.ID).
						WithKey("balance").
						WithReferenceDate(lastMonth).
						Get()

					temporal2 := stubs.NewTemporalPropertyStub().
						WithEntityID(entity.ID).
						WithKey("transactions").
						WithReferenceDate(lastMonth).
						Get()

					seeder.InsertTemporalProperty(ctx, temporal1)
					seeder.InsertTemporalProperty(ctx, temporal2)

					// ACT
					result, err := graphService.GetTreeByEntityID(ctx, entity.ID, defaultDepth, lastMonth)

					// ASSERT
					Expect(err).NotTo(HaveOccurred())
					Expect(result.TemporalData).To(HaveLen(2))
					Expect(result.TemporalData).To(ContainElements(
						BeComparableTo(temporal1, comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
						BeComparableTo(temporal2, comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
					))
				})
			})
		})

		Context("entity with multiple temporal data in last three months", func() {

			When("requesting temporal data from current month", func() {
				It("should return temporal data from current month", func() {
					// ARRANGE
					entity := stubs.NewEntityStub().Get()
					seeder.InsertEntity(ctx, &entity)

					// Criar dados temporais para o mês passado
					temporal1 := stubs.NewTemporalPropertyStub().
						WithEntityID(entity.ID).
						WithKey("balance").
						WithReferenceDate(currentMonth).
						Get()

					temporal2 := stubs.NewTemporalPropertyStub().
						WithEntityID(entity.ID).
						WithKey("transactions").
						WithReferenceDate(currentMonth).
						Get()

					temporal3 := stubs.NewTemporalPropertyStub().
						WithEntityID(entity.ID).
						WithKey("balance").
						WithReferenceDate(lastMonth).
						Get()

					temporal4 := stubs.NewTemporalPropertyStub().
						WithEntityID(entity.ID).
						WithKey("transactions").
						WithReferenceDate(lastMonth).
						Get()

					temporal5 := stubs.NewTemporalPropertyStub().
						WithEntityID(entity.ID).
						WithKey("balance").
						WithReferenceDate(twoMonthsAgo).
						Get()

					temporal6 := stubs.NewTemporalPropertyStub().
						WithEntityID(entity.ID).
						WithKey("transactions").
						WithReferenceDate(twoMonthsAgo).
						Get()

					seeder.InsertTemporalProperty(ctx, temporal1)
					seeder.InsertTemporalProperty(ctx, temporal2)
					seeder.InsertTemporalProperty(ctx, temporal3)
					seeder.InsertTemporalProperty(ctx, temporal4)
					seeder.InsertTemporalProperty(ctx, temporal5)
					seeder.InsertTemporalProperty(ctx, temporal6)

					// ACT
					result, err := graphService.GetTreeByEntityID(ctx, entity.ID, defaultDepth, currentMonth)

					// ASSERT
					Expect(err).NotTo(HaveOccurred())
					Expect(result.TemporalData).To(HaveLen(2))
					Expect(result.TemporalData).To(ContainElements(
						BeComparableTo(temporal1, comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
						BeComparableTo(temporal2, comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
					))
				})
			})

			When("requesting temporal data from last month onwards", func() {
				It("should return temporal data from last month and current", func() {
					// ARRANGE
					entity := stubs.NewEntityStub().Get()
					seeder.InsertEntity(ctx, &entity)

					// Criar dados temporais para o mês passado
					temporal1 := stubs.NewTemporalPropertyStub().
						WithEntityID(entity.ID).
						WithKey("balance").
						WithReferenceDate(currentMonth).
						Get()

					temporal2 := stubs.NewTemporalPropertyStub().
						WithEntityID(entity.ID).
						WithKey("transactions").
						WithReferenceDate(currentMonth).
						Get()

					temporal3 := stubs.NewTemporalPropertyStub().
						WithEntityID(entity.ID).
						WithKey("balance").
						WithReferenceDate(lastMonth).
						Get()

					temporal4 := stubs.NewTemporalPropertyStub().
						WithEntityID(entity.ID).
						WithKey("transactions").
						WithReferenceDate(lastMonth).
						Get()

					temporal5 := stubs.NewTemporalPropertyStub().
						WithEntityID(entity.ID).
						WithKey("balance").
						WithReferenceDate(twoMonthsAgo).
						Get()

					temporal6 := stubs.NewTemporalPropertyStub().
						WithEntityID(entity.ID).
						WithKey("transactions").
						WithReferenceDate(twoMonthsAgo).
						Get()

					seeder.InsertTemporalProperty(ctx, temporal1)
					seeder.InsertTemporalProperty(ctx, temporal2)
					seeder.InsertTemporalProperty(ctx, temporal3)
					seeder.InsertTemporalProperty(ctx, temporal4)
					seeder.InsertTemporalProperty(ctx, temporal5)
					seeder.InsertTemporalProperty(ctx, temporal6)

					// ACT
					result, err := graphService.GetTreeByEntityID(ctx, entity.ID, defaultDepth, lastMonth)

					// ASSERT
					Expect(err).NotTo(HaveOccurred())
					Expect(result.TemporalData).To(HaveLen(4))
					Expect(result.TemporalData).To(ContainElements(
						BeComparableTo(temporal1, comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
						BeComparableTo(temporal2, comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
						BeComparableTo(temporal3, comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
						BeComparableTo(temporal4, comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
					))
				})
			})

			When("requesting temporal data from two months ago onwards", func() {
				It("should return temporal data from two months ago, last month and current", func() {
					// ARRANGE
					entity := stubs.NewEntityStub().Get()
					seeder.InsertEntity(ctx, &entity)

					// Criar dados temporais para o mês passado
					temporal1 := stubs.NewTemporalPropertyStub().
						WithEntityID(entity.ID).
						WithKey("balance").
						WithReferenceDate(currentMonth).
						Get()

					temporal2 := stubs.NewTemporalPropertyStub().
						WithEntityID(entity.ID).
						WithKey("transactions").
						WithReferenceDate(currentMonth).
						Get()

					temporal3 := stubs.NewTemporalPropertyStub().
						WithEntityID(entity.ID).
						WithKey("balance").
						WithReferenceDate(lastMonth).
						Get()

					temporal4 := stubs.NewTemporalPropertyStub().
						WithEntityID(entity.ID).
						WithKey("transactions").
						WithReferenceDate(lastMonth).
						Get()

					temporal5 := stubs.NewTemporalPropertyStub().
						WithEntityID(entity.ID).
						WithKey("balance").
						WithReferenceDate(twoMonthsAgo).
						Get()

					temporal6 := stubs.NewTemporalPropertyStub().
						WithEntityID(entity.ID).
						WithKey("transactions").
						WithReferenceDate(twoMonthsAgo).
						Get()

					seeder.InsertTemporalProperty(ctx, temporal1)
					seeder.InsertTemporalProperty(ctx, temporal2)
					seeder.InsertTemporalProperty(ctx, temporal3)
					seeder.InsertTemporalProperty(ctx, temporal4)
					seeder.InsertTemporalProperty(ctx, temporal5)
					seeder.InsertTemporalProperty(ctx, temporal6)

					// ACT
					result, err := graphService.GetTreeByEntityID(ctx, entity.ID, defaultDepth, twoMonthsAgo)

					// ASSERT
					Expect(err).NotTo(HaveOccurred())
					Expect(result.TemporalData).To(HaveLen(6))
					Expect(result.TemporalData).To(ContainElements(
						BeComparableTo(temporal1, comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
						BeComparableTo(temporal2, comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
						BeComparableTo(temporal3, comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
						BeComparableTo(temporal4, comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
						BeComparableTo(temporal5, comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
						BeComparableTo(temporal6, comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
					))
				})

			})

			When("requesting temporal data from five months ago", func() {
				It("should return temporal data from last 3 months", func() {
					// ARRANGE
					entity := stubs.NewEntityStub().Get()
					seeder.InsertEntity(ctx, &entity)

					// Criar dados temporais para o mês passado
					temporal1 := stubs.NewTemporalPropertyStub().
						WithEntityID(entity.ID).
						WithKey("balance").
						WithReferenceDate(currentMonth).
						Get()

					temporal2 := stubs.NewTemporalPropertyStub().
						WithEntityID(entity.ID).
						WithKey("transactions").
						WithReferenceDate(currentMonth).
						Get()

					temporal3 := stubs.NewTemporalPropertyStub().
						WithEntityID(entity.ID).
						WithKey("balance").
						WithReferenceDate(lastMonth).
						Get()

					temporal4 := stubs.NewTemporalPropertyStub().
						WithEntityID(entity.ID).
						WithKey("transactions").
						WithReferenceDate(lastMonth).
						Get()

					temporal5 := stubs.NewTemporalPropertyStub().
						WithEntityID(entity.ID).
						WithKey("balance").
						WithReferenceDate(twoMonthsAgo).
						Get()

					temporal6 := stubs.NewTemporalPropertyStub().
						WithEntityID(entity.ID).
						WithKey("transactions").
						WithReferenceDate(twoMonthsAgo).
						Get()

					seeder.InsertTemporalProperty(ctx, temporal1)
					seeder.InsertTemporalProperty(ctx, temporal2)
					seeder.InsertTemporalProperty(ctx, temporal3)
					seeder.InsertTemporalProperty(ctx, temporal4)
					seeder.InsertTemporalProperty(ctx, temporal5)
					seeder.InsertTemporalProperty(ctx, temporal6)

					fiveMonthsAgo := time.Now().AddDate(0, -5, 0)

					// ACT
					result, err := graphService.GetTreeByEntityID(ctx, entity.ID, defaultDepth, fiveMonthsAgo)

					// ASSERT
					Expect(err).NotTo(HaveOccurred())
					Expect(result.TemporalData).To(HaveLen(6))
					Expect(result.TemporalData).To(ContainElements(
						BeComparableTo(temporal1, comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
						BeComparableTo(temporal2, comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
						BeComparableTo(temporal3, comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
						BeComparableTo(temporal4, comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
						BeComparableTo(temporal5, comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
						BeComparableTo(temporal6, comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
					))
				})
			})
		})
	})

	Context("Search with temporal data and relationships", func() {
		Context("entity with relationships and temporal data", func() {
			When("searching entity with temporal data and nested edges", func() {
				It("should return complete structure with temporal data at correct levels", func() {
					// ARRANGE
					rootEntity := stubs.NewEntityStub().Get()
					childEntity := stubs.NewEntityStub().Get()
					grandchildEntity := stubs.NewEntityStub().Get()

					seeder.InsertEntity(ctx, &rootEntity)
					seeder.InsertEntity(ctx, &childEntity)
					seeder.InsertEntity(ctx, &grandchildEntity)

					// Relacionamentos
					edge1 := stubs.NewEdgeStub().WithLeftEntityID(rootEntity.ID).WithRightEntityID(childEntity.ID).Get()
					edge2 := stubs.NewEdgeStub().WithLeftEntityID(childEntity.ID).WithRightEntityID(grandchildEntity.ID).Get()

					seeder.InsertEdge(ctx, &edge1)
					seeder.InsertEdge(ctx, &edge2)

					// Dados temporais para diferentes níveis
					rootTemporal := stubs.NewTemporalPropertyStub().
						WithEntityID(rootEntity.ID).
						WithKey("balance").
						WithReferenceDate(currentMonth).
						Get()

					childTemporal := stubs.NewTemporalPropertyStub().
						WithEntityID(childEntity.ID).
						WithKey("transactions").
						WithReferenceDate(currentMonth).
						Get()

					grandchildTemporal := stubs.NewTemporalPropertyStub().
						WithEntityID(grandchildEntity.ID).
						WithKey("score").
						WithReferenceDate(currentMonth).
						Get()

					seeder.InsertTemporalProperty(ctx, rootTemporal)
					seeder.InsertTemporalProperty(ctx, childTemporal)
					seeder.InsertTemporalProperty(ctx, grandchildTemporal)

					expected := &domain.NodeTree{
						Entity:       rootEntity,
						TemporalData: []entities.TemporalProperty{rootTemporal},
						Edges: []*domain.ProfileEdge{
							&domain.ProfileEdge{
								Type: edge1.RelationshipType,
								Entity: &domain.NodeTree{
									Entity:       childEntity,
									TemporalData: []entities.TemporalProperty{childTemporal},
									Edges: []*domain.ProfileEdge{
										&domain.ProfileEdge{
											Type: edge2.RelationshipType,
											Entity: &domain.NodeTree{
												Entity:       grandchildEntity,
												TemporalData: []entities.TemporalProperty{grandchildTemporal},
												Edges:        []*domain.ProfileEdge{},
											},
										},
									},
								},
							},
						},
					}

					// ACT
					result, err := graphService.GetTreeByEntityID(ctx, rootEntity.ID, 2, currentMonth)

					// ASSERT
					Expect(err).NotTo(HaveOccurred())
					Expect(result).To(BeComparableTo(
						expected,
						comparer.TimeWithinTolerance(200),
						comparer.JSONRawMessage(),
					))
				})
			})

			When("searching with limited depth and temporal data from different months", func() {
				It("should return only temporal data from requested month at all levels", func() {
					// ARRANGE
					rootEntity := stubs.NewEntityStub().Get()
					childEntity := stubs.NewEntityStub().Get()

					seeder.InsertEntity(ctx, &rootEntity)
					seeder.InsertEntity(ctx, &childEntity)

					// Relacionamento
					edge := stubs.NewEdgeStub().WithLeftEntityID(rootEntity.ID).WithRightEntityID(childEntity.ID).Get()
					seeder.InsertEdge(ctx, &edge)

					// Dados temporais para mês atual
					rootTemporalCurrent := stubs.NewTemporalPropertyStub().
						WithEntityID(rootEntity.ID).
						WithKey("balance").
						WithReferenceDate(currentMonth).
						Get()

					childTemporalCurrent := stubs.NewTemporalPropertyStub().
						WithEntityID(childEntity.ID).
						WithKey("transactions").
						WithReferenceDate(currentMonth).
						Get()

					// Dados temporais para mês passado (não devem aparecer)
					rootTemporalLast := stubs.NewTemporalPropertyStub().
						WithEntityID(rootEntity.ID).
						WithKey("old_balance").
						WithReferenceDate(lastMonth).
						Get()

					childTemporalLast := stubs.NewTemporalPropertyStub().
						WithEntityID(childEntity.ID).
						WithKey("old_transactions").
						WithReferenceDate(lastMonth).
						Get()

					seeder.InsertTemporalProperty(ctx, rootTemporalCurrent)
					seeder.InsertTemporalProperty(ctx, childTemporalCurrent)
					seeder.InsertTemporalProperty(ctx, rootTemporalLast)
					seeder.InsertTemporalProperty(ctx, childTemporalLast)

					expected := &domain.NodeTree{
						Entity:       rootEntity,
						TemporalData: []entities.TemporalProperty{rootTemporalCurrent},
						Edges: []*domain.ProfileEdge{
							&domain.ProfileEdge{
								Type: edge.RelationshipType,
								Entity: &domain.NodeTree{
									Entity:       childEntity,
									TemporalData: []entities.TemporalProperty{childTemporalCurrent},
									Edges:        []*domain.ProfileEdge{},
								},
							},
						},
					}

					// ACT
					result, err := graphService.GetTreeByEntityID(ctx, rootEntity.ID, 1, currentMonth)

					// ASSERT
					Expect(err).NotTo(HaveOccurred())
					Expect(result).To(BeComparableTo(
						expected,
						comparer.TimeWithinTolerance(200),
						comparer.JSONRawMessage(),
					))
				})
			})
		})
	})
})
