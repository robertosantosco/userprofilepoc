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

var _ = Describe("GetTreesByEntityIDs", func() {
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
		Context("no entities exist in database", func() {
			When("searching for any ids", func() {
				It("should return empty list", func() {
					// ARRANGE
					requestedIDs := []int64{999, 1000, 1001}

					// ACT
					trees := graphService.GetTreesByEntityIDs(ctx, requestedIDs, defaultDepth, defaultReferenceDate)

					// ASSERT
					Expect(trees).To(BeEmpty())
				})
			})
		})

		Context("only one entity exists in database", func() {
			When("searching for ids that do not exist", func() {
				It("should return empty list", func() {
					// ARRANGE
					existingEntity := stubs.NewEntityStub().Get()
					seeder.InsertEntity(ctx, &existingEntity)
					requestedIDs := []int64{999, 1000, 1001}

					// ACT
					trees := graphService.GetTreesByEntityIDs(ctx, requestedIDs, defaultDepth, defaultReferenceDate)

					// ASSERT
					Expect(trees).To(BeEmpty())
				})
			})

			When("searching for existing id only", func() {
				It("should return expected domain.NodeTree list", func() {
					// ARRANGE
					existingEntity := stubs.NewEntityStub().Get()
					seeder.InsertEntity(ctx, &existingEntity)
					requestedIDs := []int64{existingEntity.ID}

					expected := []*domain.NodeTree{
						&domain.NodeTree{
							Entity:       existingEntity,
							TemporalData: nil,
							Edges:        []*domain.ProfileEdge{},
						},
					}

					// ACT
					trees := graphService.GetTreesByEntityIDs(ctx, requestedIDs, defaultDepth, defaultReferenceDate)

					// ASSERT
					Expect(trees).To(BeComparableTo(
						expected,
						comparer.TimeWithinTolerance(200),
						comparer.JSONRawMessage(),
					))
				})
			})

			When("searching for mix of existing and non-existing ids", func() {
				It("should return only existing entities", func() {
					// ARRANGE
					existingEntity := stubs.NewEntityStub().Get()
					seeder.InsertEntity(ctx, &existingEntity)
					requestedIDs := []int64{existingEntity.ID, 999, 1000}

					expected := []*domain.NodeTree{
						&domain.NodeTree{
							Entity:       existingEntity,
							TemporalData: nil,
							Edges:        []*domain.ProfileEdge{},
						},
					}

					// ACT
					trees := graphService.GetTreesByEntityIDs(ctx, requestedIDs, defaultDepth, defaultReferenceDate)

					// ASSERT
					Expect(trees).To(BeComparableTo(
						expected,
						comparer.TimeWithinTolerance(200),
						comparer.JSONRawMessage(),
					))
				})
			})
		})

		Context("multiple entities exist in database", func() {
			When("searching for ids that do not exist", func() {
				It("should return empty list", func() {
					// ARRANGE
					entity1 := stubs.NewEntityStub().Get()
					entity2 := stubs.NewEntityStub().Get()
					entity3 := stubs.NewEntityStub().Get()

					seeder.InsertEntity(ctx, &entity1)
					seeder.InsertEntity(ctx, &entity2)
					seeder.InsertEntity(ctx, &entity3)

					requestedIDs := []int64{999, 1000, 1001}

					// ACT
					trees := graphService.GetTreesByEntityIDs(ctx, requestedIDs, defaultDepth, defaultReferenceDate)

					// ASSERT
					Expect(trees).To(BeEmpty())
				})
			})

			When("searching for existing ids", func() {
				It("should return expected domain.NodeTree list", func() {
					// ARRANGE
					entity1 := stubs.NewEntityStub().Get()
					entity2 := stubs.NewEntityStub().Get()
					entity3 := stubs.NewEntityStub().Get()

					seeder.InsertEntity(ctx, &entity1)
					seeder.InsertEntity(ctx, &entity2)
					seeder.InsertEntity(ctx, &entity3)

					requestedIDs := []int64{entity1.ID, entity3.ID}

					expected := []*domain.NodeTree{
						&domain.NodeTree{
							Entity:       entity1,
							TemporalData: nil,
							Edges:        []*domain.ProfileEdge{},
						},
						&domain.NodeTree{
							Entity:       entity3,
							TemporalData: nil,
							Edges:        []*domain.ProfileEdge{},
						},
					}

					// ACT
					trees := graphService.GetTreesByEntityIDs(ctx, requestedIDs, defaultDepth, defaultReferenceDate)

					// ASSERT
					Expect(trees).To(ContainElements(
						BeComparableTo(expected[0], comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
						BeComparableTo(expected[1], comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
					))
				})
			})

			When("searching for mix of existing and non-existing ids", func() {
				It("should return only existing entities", func() {
					// ARRANGE
					entity1 := stubs.NewEntityStub().Get()
					entity2 := stubs.NewEntityStub().Get()
					entity3 := stubs.NewEntityStub().Get()

					seeder.InsertEntity(ctx, &entity1)
					seeder.InsertEntity(ctx, &entity2)
					seeder.InsertEntity(ctx, &entity3)

					requestedIDs := []int64{entity1.ID, 999, entity3.ID, 1000}

					expected := []*domain.NodeTree{
						&domain.NodeTree{
							Entity:       entity1,
							TemporalData: nil,
							Edges:        []*domain.ProfileEdge{},
						},
						&domain.NodeTree{
							Entity:       entity3,
							TemporalData: nil,
							Edges:        []*domain.ProfileEdge{},
						},
					}

					// ACT
					trees := graphService.GetTreesByEntityIDs(ctx, requestedIDs, defaultDepth, defaultReferenceDate)

					// ASSERT
					Expect(trees).To(ContainElements(
						BeComparableTo(expected[0], comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
						BeComparableTo(expected[1], comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
					))
				})
			})
		})
	})

	Context("Search with depth", func() {
		Context("entities with multiple levels", func() {
			When("searching with lower number of levels", func() {
				It("should return only entities from requested level in edges", func() {
					// ARRANGE
					rootEntity1 := stubs.NewEntityStub().Get()
					rootEntity2 := stubs.NewEntityStub().Get()
					childEntity1 := stubs.NewEntityStub().Get()
					childEntity2 := stubs.NewEntityStub().Get()
					grandchildEntity := stubs.NewEntityStub().Get()

					seeder.InsertEntity(ctx, &rootEntity1)
					seeder.InsertEntity(ctx, &rootEntity2)
					seeder.InsertEntity(ctx, &childEntity1)
					seeder.InsertEntity(ctx, &childEntity2)
					seeder.InsertEntity(ctx, &grandchildEntity)

					edge1 := stubs.NewEdgeStub().WithLeftEntityID(rootEntity1.ID).WithRightEntityID(childEntity1.ID).Get()
					edge2 := stubs.NewEdgeStub().WithLeftEntityID(rootEntity2.ID).WithRightEntityID(childEntity2.ID).Get()
					edge3 := stubs.NewEdgeStub().WithLeftEntityID(childEntity1.ID).WithRightEntityID(grandchildEntity.ID).Get()

					seeder.InsertEdge(ctx, &edge1)
					seeder.InsertEdge(ctx, &edge2)
					seeder.InsertEdge(ctx, &edge3)

					requestedIDs := []int64{rootEntity1.ID, rootEntity2.ID}

					expected := []*domain.NodeTree{
						&domain.NodeTree{
							Entity:       rootEntity1,
							TemporalData: nil,
							Edges: []*domain.ProfileEdge{
								&domain.ProfileEdge{
									Type: edge1.RelationshipType,
									Entity: &domain.NodeTree{
										Entity:       childEntity1,
										TemporalData: nil,
										Edges:        []*domain.ProfileEdge{},
									},
								},
							},
						},
						&domain.NodeTree{
							Entity:       rootEntity2,
							TemporalData: nil,
							Edges: []*domain.ProfileEdge{
								&domain.ProfileEdge{
									Type: edge2.RelationshipType,
									Entity: &domain.NodeTree{
										Entity:       childEntity2,
										TemporalData: nil,
										Edges:        []*domain.ProfileEdge{},
									},
								},
							},
						},
					}

					// ACT
					trees := graphService.GetTreesByEntityIDs(ctx, requestedIDs, 1, defaultReferenceDate)

					// ASSERT
					Expect(trees).To(ContainElements(
						BeComparableTo(expected[0], comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
						BeComparableTo(expected[1], comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
					))
				})
			})

			When("searching with equal number of levels", func() {
				It("should return all associated entities in edges", func() {
					// ARRANGE
					rootEntity1 := stubs.NewEntityStub().Get()
					rootEntity2 := stubs.NewEntityStub().Get()
					childEntity1 := stubs.NewEntityStub().Get()
					childEntity2 := stubs.NewEntityStub().Get()
					grandchildEntity := stubs.NewEntityStub().Get()

					seeder.InsertEntity(ctx, &rootEntity1)
					seeder.InsertEntity(ctx, &rootEntity2)
					seeder.InsertEntity(ctx, &childEntity1)
					seeder.InsertEntity(ctx, &childEntity2)
					seeder.InsertEntity(ctx, &grandchildEntity)

					edge1 := stubs.NewEdgeStub().WithLeftEntityID(rootEntity1.ID).WithRightEntityID(childEntity1.ID).Get()
					edge2 := stubs.NewEdgeStub().WithLeftEntityID(rootEntity2.ID).WithRightEntityID(childEntity2.ID).Get()
					edge3 := stubs.NewEdgeStub().WithLeftEntityID(childEntity1.ID).WithRightEntityID(grandchildEntity.ID).Get()

					seeder.InsertEdge(ctx, &edge1)
					seeder.InsertEdge(ctx, &edge2)
					seeder.InsertEdge(ctx, &edge3)

					requestedIDs := []int64{rootEntity1.ID, rootEntity2.ID}

					expected := []*domain.NodeTree{
						&domain.NodeTree{
							Entity:       rootEntity1,
							TemporalData: nil,
							Edges: []*domain.ProfileEdge{
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
							},
						},
						&domain.NodeTree{
							Entity:       rootEntity2,
							TemporalData: nil,
							Edges: []*domain.ProfileEdge{
								&domain.ProfileEdge{
									Type: edge2.RelationshipType,
									Entity: &domain.NodeTree{
										Entity:       childEntity2,
										TemporalData: nil,
										Edges:        []*domain.ProfileEdge{},
									},
								},
							},
						},
					}

					// ACT
					trees := graphService.GetTreesByEntityIDs(ctx, requestedIDs, 2, defaultReferenceDate)

					// ASSERT
					Expect(trees).To(ContainElements(
						BeComparableTo(expected[0], comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
						BeComparableTo(expected[1], comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
					))
				})
			})

			When("searching with higher number of levels", func() {
				It("should return only associated entities in edges", func() {
					// ARRANGE
					rootEntity1 := stubs.NewEntityStub().Get()
					rootEntity2 := stubs.NewEntityStub().Get()
					childEntity1 := stubs.NewEntityStub().Get()
					childEntity2 := stubs.NewEntityStub().Get()
					grandchildEntity := stubs.NewEntityStub().Get()

					seeder.InsertEntity(ctx, &rootEntity1)
					seeder.InsertEntity(ctx, &rootEntity2)
					seeder.InsertEntity(ctx, &childEntity1)
					seeder.InsertEntity(ctx, &childEntity2)
					seeder.InsertEntity(ctx, &grandchildEntity)

					edge1 := stubs.NewEdgeStub().WithLeftEntityID(rootEntity1.ID).WithRightEntityID(childEntity1.ID).Get()
					edge2 := stubs.NewEdgeStub().WithLeftEntityID(rootEntity2.ID).WithRightEntityID(childEntity2.ID).Get()
					edge3 := stubs.NewEdgeStub().WithLeftEntityID(childEntity1.ID).WithRightEntityID(grandchildEntity.ID).Get()

					seeder.InsertEdge(ctx, &edge1)
					seeder.InsertEdge(ctx, &edge2)
					seeder.InsertEdge(ctx, &edge3)

					requestedIDs := []int64{rootEntity1.ID, rootEntity2.ID}

					expected := []*domain.NodeTree{
						&domain.NodeTree{
							Entity:       rootEntity1,
							TemporalData: nil,
							Edges: []*domain.ProfileEdge{
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
							},
						},
						&domain.NodeTree{
							Entity:       rootEntity2,
							TemporalData: nil,
							Edges: []*domain.ProfileEdge{
								&domain.ProfileEdge{
									Type: edge2.RelationshipType,
									Entity: &domain.NodeTree{
										Entity:       childEntity2,
										TemporalData: nil,
										Edges:        []*domain.ProfileEdge{},
									},
								},
							},
						},
					}

					// ACT
					trees := graphService.GetTreesByEntityIDs(ctx, requestedIDs, 5, defaultReferenceDate)

					// ASSERT
					Expect(trees).To(ContainElements(
						BeComparableTo(expected[0], comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
						BeComparableTo(expected[1], comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
					))
				})
			})
		})

		Context("entities have no levels", func() {
			When("searching with any depth", func() {
				It("should return empty list in edges", func() {
					// ARRANGE
					entity1 := stubs.NewEntityStub().Get()
					entity2 := stubs.NewEntityStub().Get()
					seeder.InsertEntity(ctx, &entity1)
					seeder.InsertEntity(ctx, &entity2)

					requestedIDs := []int64{entity1.ID, entity2.ID}

					expected := []*domain.NodeTree{
						&domain.NodeTree{
							Entity:       entity1,
							TemporalData: nil,
							Edges:        []*domain.ProfileEdge{},
						},
						&domain.NodeTree{
							Entity:       entity2,
							TemporalData: nil,
							Edges:        []*domain.ProfileEdge{},
						},
					}

					// ACT
					trees := graphService.GetTreesByEntityIDs(ctx, requestedIDs, defaultDepth, defaultReferenceDate)

					// ASSERT
					Expect(trees).To(ContainElements(
						BeComparableTo(expected[0], comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
						BeComparableTo(expected[1], comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
					))
				})
			})
		})
	})

	Context("Search with temporal data", func() {
		Context("entities with multiple temporal data for last month", func() {
			When("requesting temporal data from current month", func() {
				It("should return empty list in temporal data", func() {
					// ARRANGE
					entity1 := stubs.NewEntityStub().Get()
					entity2 := stubs.NewEntityStub().Get()
					seeder.InsertEntity(ctx, &entity1)
					seeder.InsertEntity(ctx, &entity2)

					// Criar dados temporais para o mês passado
					temporal1 := stubs.NewTemporalPropertyStub().
						WithEntityID(entity1.ID).
						WithKey("balance").
						WithReferenceDate(lastMonth).
						Get()

					temporal2 := stubs.NewTemporalPropertyStub().
						WithEntityID(entity2.ID).
						WithKey("transactions").
						WithReferenceDate(lastMonth).
						Get()

					seeder.InsertTemporalProperty(ctx, temporal1)
					seeder.InsertTemporalProperty(ctx, temporal2)

					requestedIDs := []int64{entity1.ID, entity2.ID}

					expected := []*domain.NodeTree{
						&domain.NodeTree{
							Entity:       entity1,
							TemporalData: nil,
							Edges:        []*domain.ProfileEdge{},
						},
						&domain.NodeTree{
							Entity:       entity2,
							TemporalData: nil,
							Edges:        []*domain.ProfileEdge{},
						},
					}

					// ACT
					trees := graphService.GetTreesByEntityIDs(ctx, requestedIDs, defaultDepth, currentMonth)

					// ASSERT
					Expect(trees).To(ContainElements(
						BeComparableTo(expected[0], comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
						BeComparableTo(expected[1], comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
					))
				})
			})

			When("requesting temporal data from last month", func() {
				It("should return temporal data", func() {
					// ARRANGE
					entity1 := stubs.NewEntityStub().Get()
					entity2 := stubs.NewEntityStub().Get()
					seeder.InsertEntity(ctx, &entity1)
					seeder.InsertEntity(ctx, &entity2)

					// Criar dados temporais para o mês passado
					temporal1 := stubs.NewTemporalPropertyStub().
						WithEntityID(entity1.ID).
						WithKey("balance").
						WithReferenceDate(lastMonth).
						Get()

					temporal2 := stubs.NewTemporalPropertyStub().
						WithEntityID(entity2.ID).
						WithKey("transactions").
						WithReferenceDate(lastMonth).
						Get()

					seeder.InsertTemporalProperty(ctx, temporal1)
					seeder.InsertTemporalProperty(ctx, temporal2)

					requestedIDs := []int64{entity1.ID, entity2.ID}

					expected := []*domain.NodeTree{
						&domain.NodeTree{
							Entity:       entity1,
							TemporalData: []entities.TemporalProperty{temporal1},
							Edges:        []*domain.ProfileEdge{},
						},
						&domain.NodeTree{
							Entity:       entity2,
							TemporalData: []entities.TemporalProperty{temporal2},
							Edges:        []*domain.ProfileEdge{},
						},
					}

					// ACT
					trees := graphService.GetTreesByEntityIDs(ctx, requestedIDs, defaultDepth, lastMonth)

					// ASSERT
					Expect(trees).To(ContainElements(
						BeComparableTo(expected[0], comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
						BeComparableTo(expected[1], comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
					))
				})
			})
		})

		Context("entities with multiple temporal data in last three months", func() {

			When("requesting temporal data from current month", func() {
				It("should return temporal data from current month", func() {
					// ARRANGE
					entity1 := stubs.NewEntityStub().Get()
					entity2 := stubs.NewEntityStub().Get()
					seeder.InsertEntity(ctx, &entity1)
					seeder.InsertEntity(ctx, &entity2)

					// Criar dados temporais para o mês atual
					temporal1Current := stubs.NewTemporalPropertyStub().
						WithEntityID(entity1.ID).
						WithKey("balance").
						WithReferenceDate(currentMonth).
						Get()

					temporal2Current := stubs.NewTemporalPropertyStub().
						WithEntityID(entity2.ID).
						WithKey("transactions").
						WithReferenceDate(currentMonth).
						Get()

					// Criar dados temporais para o mês passado
					temporal1Last := stubs.NewTemporalPropertyStub().
						WithEntityID(entity1.ID).
						WithKey("balance").
						WithReferenceDate(lastMonth).
						Get()

					temporal2Last := stubs.NewTemporalPropertyStub().
						WithEntityID(entity2.ID).
						WithKey("transactions").
						WithReferenceDate(lastMonth).
						Get()

					seeder.InsertTemporalProperty(ctx, temporal1Current)
					seeder.InsertTemporalProperty(ctx, temporal2Current)
					seeder.InsertTemporalProperty(ctx, temporal1Last)
					seeder.InsertTemporalProperty(ctx, temporal2Last)

					requestedIDs := []int64{entity1.ID, entity2.ID}

					expected := []*domain.NodeTree{
						&domain.NodeTree{
							Entity:       entity1,
							TemporalData: []entities.TemporalProperty{temporal1Current},
							Edges:        []*domain.ProfileEdge{},
						},
						&domain.NodeTree{
							Entity:       entity2,
							TemporalData: []entities.TemporalProperty{temporal2Current},
							Edges:        []*domain.ProfileEdge{},
						},
					}

					// ACT
					trees := graphService.GetTreesByEntityIDs(ctx, requestedIDs, defaultDepth, currentMonth)

					// ASSERT
					Expect(trees).To(ContainElements(
						BeComparableTo(expected[0], comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
						BeComparableTo(expected[1], comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
					))
				})
			})

			When("requesting temporal data from last month onwards", func() {
				It("should return temporal data from last month and current", func() {
					// ARRANGE
					entity1 := stubs.NewEntityStub().Get()
					entity2 := stubs.NewEntityStub().Get()
					seeder.InsertEntity(ctx, &entity1)
					seeder.InsertEntity(ctx, &entity2)

					// Criar dados temporais para o mês atual
					temporal1Current := stubs.NewTemporalPropertyStub().
						WithEntityID(entity1.ID).
						WithKey("balance").
						WithReferenceDate(currentMonth).
						Get()

					temporal2Current := stubs.NewTemporalPropertyStub().
						WithEntityID(entity2.ID).
						WithKey("transactions").
						WithReferenceDate(currentMonth).
						Get()

					// Criar dados temporais para o mês passado
					temporal1Last := stubs.NewTemporalPropertyStub().
						WithEntityID(entity1.ID).
						WithKey("balance").
						WithReferenceDate(lastMonth).
						Get()

					temporal2Last := stubs.NewTemporalPropertyStub().
						WithEntityID(entity2.ID).
						WithKey("transactions").
						WithReferenceDate(lastMonth).
						Get()

					seeder.InsertTemporalProperty(ctx, temporal1Current)
					seeder.InsertTemporalProperty(ctx, temporal2Current)
					seeder.InsertTemporalProperty(ctx, temporal1Last)
					seeder.InsertTemporalProperty(ctx, temporal2Last)

					requestedIDs := []int64{entity1.ID, entity2.ID}

					expected := []*domain.NodeTree{
						&domain.NodeTree{
							Entity:       entity1,
							TemporalData: []entities.TemporalProperty{temporal1Current, temporal1Last},
							Edges:        []*domain.ProfileEdge{},
						},
						&domain.NodeTree{
							Entity:       entity2,
							TemporalData: []entities.TemporalProperty{temporal2Current, temporal2Last},
							Edges:        []*domain.ProfileEdge{},
						},
					}

					// ACT
					trees := graphService.GetTreesByEntityIDs(ctx, requestedIDs, defaultDepth, lastMonth)

					// ASSERT
					Expect(trees).To(ContainElements(
						BeComparableTo(expected[0], comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
						BeComparableTo(expected[1], comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
					))
				})
			})

			When("requesting temporal data from two months ago onwards", func() {
				It("should return temporal data from two months ago, last month and current", func() {
					// ARRANGE
					entity1 := stubs.NewEntityStub().Get()
					entity2 := stubs.NewEntityStub().Get()
					seeder.InsertEntity(ctx, &entity1)
					seeder.InsertEntity(ctx, &entity2)

					// Criar dados temporais para o mês atual
					temporal1Current := stubs.NewTemporalPropertyStub().
						WithEntityID(entity1.ID).
						WithKey("balance").
						WithReferenceDate(currentMonth).
						Get()

					temporal2Current := stubs.NewTemporalPropertyStub().
						WithEntityID(entity2.ID).
						WithKey("transactions").
						WithReferenceDate(currentMonth).
						Get()

					// Criar dados temporais para o mês passado
					temporal1Last := stubs.NewTemporalPropertyStub().
						WithEntityID(entity1.ID).
						WithKey("balance").
						WithReferenceDate(lastMonth).
						Get()

					temporal2Last := stubs.NewTemporalPropertyStub().
						WithEntityID(entity2.ID).
						WithKey("transactions").
						WithReferenceDate(lastMonth).
						Get()

					// Criar dados temporais para dois meses atrás
					temporal1TwoMonths := stubs.NewTemporalPropertyStub().
						WithEntityID(entity1.ID).
						WithKey("balance").
						WithReferenceDate(twoMonthsAgo).
						Get()

					temporal2TwoMonths := stubs.NewTemporalPropertyStub().
						WithEntityID(entity2.ID).
						WithKey("transactions").
						WithReferenceDate(twoMonthsAgo).
						Get()

					seeder.InsertTemporalProperty(ctx, temporal1Current)
					seeder.InsertTemporalProperty(ctx, temporal2Current)
					seeder.InsertTemporalProperty(ctx, temporal1Last)
					seeder.InsertTemporalProperty(ctx, temporal2Last)
					seeder.InsertTemporalProperty(ctx, temporal1TwoMonths)
					seeder.InsertTemporalProperty(ctx, temporal2TwoMonths)

					requestedIDs := []int64{entity1.ID, entity2.ID}

					expected := []*domain.NodeTree{
						&domain.NodeTree{
							Entity:       entity1,
							TemporalData: []entities.TemporalProperty{temporal1Current, temporal1Last, temporal1TwoMonths},
							Edges:        []*domain.ProfileEdge{},
						},
						&domain.NodeTree{
							Entity:       entity2,
							TemporalData: []entities.TemporalProperty{temporal2Current, temporal2Last, temporal2TwoMonths},
							Edges:        []*domain.ProfileEdge{},
						},
					}

					// ACT
					trees := graphService.GetTreesByEntityIDs(ctx, requestedIDs, defaultDepth, twoMonthsAgo)

					// ASSERT
					Expect(trees).To(ContainElements(
						BeComparableTo(expected[0], comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
						BeComparableTo(expected[1], comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
					))
				})
			})

			When("requesting temporal data from five months ago", func() {
				It("should return temporal data from last 3 months", func() {
					// ARRANGE
					entity1 := stubs.NewEntityStub().Get()
					entity2 := stubs.NewEntityStub().Get()
					seeder.InsertEntity(ctx, &entity1)
					seeder.InsertEntity(ctx, &entity2)

					// Criar dados temporais para o mês atual
					temporal1Current := stubs.NewTemporalPropertyStub().
						WithEntityID(entity1.ID).
						WithKey("balance").
						WithReferenceDate(currentMonth).
						Get()

					temporal2Current := stubs.NewTemporalPropertyStub().
						WithEntityID(entity2.ID).
						WithKey("transactions").
						WithReferenceDate(currentMonth).
						Get()

					// Criar dados temporais para o mês passado
					temporal1Last := stubs.NewTemporalPropertyStub().
						WithEntityID(entity1.ID).
						WithKey("balance").
						WithReferenceDate(lastMonth).
						Get()

					temporal2Last := stubs.NewTemporalPropertyStub().
						WithEntityID(entity2.ID).
						WithKey("transactions").
						WithReferenceDate(lastMonth).
						Get()

					// Criar dados temporais para dois meses atrás
					temporal1TwoMonths := stubs.NewTemporalPropertyStub().
						WithEntityID(entity1.ID).
						WithKey("balance").
						WithReferenceDate(twoMonthsAgo).
						Get()

					temporal2TwoMonths := stubs.NewTemporalPropertyStub().
						WithEntityID(entity2.ID).
						WithKey("transactions").
						WithReferenceDate(twoMonthsAgo).
						Get()

					seeder.InsertTemporalProperty(ctx, temporal1Current)
					seeder.InsertTemporalProperty(ctx, temporal2Current)
					seeder.InsertTemporalProperty(ctx, temporal1Last)
					seeder.InsertTemporalProperty(ctx, temporal2Last)
					seeder.InsertTemporalProperty(ctx, temporal1TwoMonths)
					seeder.InsertTemporalProperty(ctx, temporal2TwoMonths)

					fiveMonthsAgo := time.Now().AddDate(0, -5, 0)
					requestedIDs := []int64{entity1.ID, entity2.ID}

					expected := []*domain.NodeTree{
						&domain.NodeTree{
							Entity:       entity1,
							TemporalData: []entities.TemporalProperty{temporal1Current, temporal1Last, temporal1TwoMonths},
							Edges:        []*domain.ProfileEdge{},
						},
						&domain.NodeTree{
							Entity:       entity2,
							TemporalData: []entities.TemporalProperty{temporal2Current, temporal2Last, temporal2TwoMonths},
							Edges:        []*domain.ProfileEdge{},
						},
					}

					// ACT
					trees := graphService.GetTreesByEntityIDs(ctx, requestedIDs, defaultDepth, fiveMonthsAgo)

					// ASSERT
					Expect(trees).To(ContainElements(
						BeComparableTo(expected[0], comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
						BeComparableTo(expected[1], comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
					))
				})
			})
		})
	})

	Context("Search with temporal data and relationships", func() {
		Context("entities with relationships and temporal data", func() {
			When("searching entities with temporal data and nested edges", func() {
				It("should return complete structure with temporal data at correct levels", func() {
					// ARRANGE
					rootEntity1 := stubs.NewEntityStub().Get()
					rootEntity2 := stubs.NewEntityStub().Get()
					childEntity1 := stubs.NewEntityStub().Get()
					childEntity2 := stubs.NewEntityStub().Get()
					grandchildEntity := stubs.NewEntityStub().Get()

					seeder.InsertEntity(ctx, &rootEntity1)
					seeder.InsertEntity(ctx, &rootEntity2)
					seeder.InsertEntity(ctx, &childEntity1)
					seeder.InsertEntity(ctx, &childEntity2)
					seeder.InsertEntity(ctx, &grandchildEntity)

					// Relacionamentos
					edge1 := stubs.NewEdgeStub().WithLeftEntityID(rootEntity1.ID).WithRightEntityID(childEntity1.ID).Get()
					edge2 := stubs.NewEdgeStub().WithLeftEntityID(rootEntity2.ID).WithRightEntityID(childEntity2.ID).Get()
					edge3 := stubs.NewEdgeStub().WithLeftEntityID(childEntity1.ID).WithRightEntityID(grandchildEntity.ID).Get()

					seeder.InsertEdge(ctx, &edge1)
					seeder.InsertEdge(ctx, &edge2)
					seeder.InsertEdge(ctx, &edge3)

					// Dados temporais para diferentes níveis
					rootTemporal1 := stubs.NewTemporalPropertyStub().
						WithEntityID(rootEntity1.ID).
						WithKey("balance").
						WithReferenceDate(currentMonth).
						Get()

					rootTemporal2 := stubs.NewTemporalPropertyStub().
						WithEntityID(rootEntity2.ID).
						WithKey("score").
						WithReferenceDate(currentMonth).
						Get()

					childTemporal1 := stubs.NewTemporalPropertyStub().
						WithEntityID(childEntity1.ID).
						WithKey("transactions").
						WithReferenceDate(currentMonth).
						Get()

					childTemporal2 := stubs.NewTemporalPropertyStub().
						WithEntityID(childEntity2.ID).
						WithKey("activity").
						WithReferenceDate(currentMonth).
						Get()

					grandchildTemporal := stubs.NewTemporalPropertyStub().
						WithEntityID(grandchildEntity.ID).
						WithKey("rating").
						WithReferenceDate(currentMonth).
						Get()

					seeder.InsertTemporalProperty(ctx, rootTemporal1)
					seeder.InsertTemporalProperty(ctx, rootTemporal2)
					seeder.InsertTemporalProperty(ctx, childTemporal1)
					seeder.InsertTemporalProperty(ctx, childTemporal2)
					seeder.InsertTemporalProperty(ctx, grandchildTemporal)

					requestedIDs := []int64{rootEntity1.ID, rootEntity2.ID}

					expected := []*domain.NodeTree{
						&domain.NodeTree{
							Entity:       rootEntity1,
							TemporalData: []entities.TemporalProperty{rootTemporal1},
							Edges: []*domain.ProfileEdge{
								&domain.ProfileEdge{
									Type: edge1.RelationshipType,
									Entity: &domain.NodeTree{
										Entity:       childEntity1,
										TemporalData: []entities.TemporalProperty{childTemporal1},
										Edges: []*domain.ProfileEdge{
											&domain.ProfileEdge{
												Type: edge3.RelationshipType,
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
						},
						&domain.NodeTree{
							Entity:       rootEntity2,
							TemporalData: []entities.TemporalProperty{rootTemporal2},
							Edges: []*domain.ProfileEdge{
								&domain.ProfileEdge{
									Type: edge2.RelationshipType,
									Entity: &domain.NodeTree{
										Entity:       childEntity2,
										TemporalData: []entities.TemporalProperty{childTemporal2},
										Edges:        []*domain.ProfileEdge{},
									},
								},
							},
						},
					}

					// ACT
					trees := graphService.GetTreesByEntityIDs(ctx, requestedIDs, 2, currentMonth)

					// ASSERT
					Expect(trees).To(ContainElements(
						BeComparableTo(expected[0], comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
						BeComparableTo(expected[1], comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
					))
				})
			})

			When("searching with limited depth and temporal data from different months", func() {
				It("should return only temporal data from requested month at all levels", func() {
					// ARRANGE
					rootEntity1 := stubs.NewEntityStub().Get()
					rootEntity2 := stubs.NewEntityStub().Get()
					childEntity1 := stubs.NewEntityStub().Get()
					childEntity2 := stubs.NewEntityStub().Get()

					seeder.InsertEntity(ctx, &rootEntity1)
					seeder.InsertEntity(ctx, &rootEntity2)
					seeder.InsertEntity(ctx, &childEntity1)
					seeder.InsertEntity(ctx, &childEntity2)

					// Relacionamentos
					edge1 := stubs.NewEdgeStub().WithLeftEntityID(rootEntity1.ID).WithRightEntityID(childEntity1.ID).Get()
					edge2 := stubs.NewEdgeStub().WithLeftEntityID(rootEntity2.ID).WithRightEntityID(childEntity2.ID).Get()

					seeder.InsertEdge(ctx, &edge1)
					seeder.InsertEdge(ctx, &edge2)

					// Dados temporais para mês atual
					rootTemporal1Current := stubs.NewTemporalPropertyStub().
						WithEntityID(rootEntity1.ID).
						WithKey("balance").
						WithReferenceDate(currentMonth).
						Get()

					rootTemporal2Current := stubs.NewTemporalPropertyStub().
						WithEntityID(rootEntity2.ID).
						WithKey("score").
						WithReferenceDate(currentMonth).
						Get()

					childTemporal1Current := stubs.NewTemporalPropertyStub().
						WithEntityID(childEntity1.ID).
						WithKey("transactions").
						WithReferenceDate(currentMonth).
						Get()

					childTemporal2Current := stubs.NewTemporalPropertyStub().
						WithEntityID(childEntity2.ID).
						WithKey("activity").
						WithReferenceDate(currentMonth).
						Get()

					// Dados temporais para mês passado (não devem aparecer)
					rootTemporal1Last := stubs.NewTemporalPropertyStub().
						WithEntityID(rootEntity1.ID).
						WithKey("old_balance").
						WithReferenceDate(lastMonth).
						Get()

					rootTemporal2Last := stubs.NewTemporalPropertyStub().
						WithEntityID(rootEntity2.ID).
						WithKey("old_score").
						WithReferenceDate(lastMonth).
						Get()

					seeder.InsertTemporalProperty(ctx, rootTemporal1Current)
					seeder.InsertTemporalProperty(ctx, rootTemporal2Current)
					seeder.InsertTemporalProperty(ctx, childTemporal1Current)
					seeder.InsertTemporalProperty(ctx, childTemporal2Current)
					seeder.InsertTemporalProperty(ctx, rootTemporal1Last)
					seeder.InsertTemporalProperty(ctx, rootTemporal2Last)

					requestedIDs := []int64{rootEntity1.ID, rootEntity2.ID}

					expected := []*domain.NodeTree{
						&domain.NodeTree{
							Entity:       rootEntity1,
							TemporalData: []entities.TemporalProperty{rootTemporal1Current},
							Edges: []*domain.ProfileEdge{
								&domain.ProfileEdge{
									Type: edge1.RelationshipType,
									Entity: &domain.NodeTree{
										Entity:       childEntity1,
										TemporalData: []entities.TemporalProperty{childTemporal1Current},
										Edges:        []*domain.ProfileEdge{},
									},
								},
							},
						},
						&domain.NodeTree{
							Entity:       rootEntity2,
							TemporalData: []entities.TemporalProperty{rootTemporal2Current},
							Edges: []*domain.ProfileEdge{
								&domain.ProfileEdge{
									Type: edge2.RelationshipType,
									Entity: &domain.NodeTree{
										Entity:       childEntity2,
										TemporalData: []entities.TemporalProperty{childTemporal2Current},
										Edges:        []*domain.ProfileEdge{},
									},
								},
							},
						},
					}

					// ACT
					trees := graphService.GetTreesByEntityIDs(ctx, requestedIDs, 1, currentMonth)

					// ASSERT
					Expect(trees).To(ContainElements(
						BeComparableTo(expected[0], comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
						BeComparableTo(expected[1], comparer.TimeWithinTolerance(200), comparer.JSONRawMessage()),
					))
				})
			})
		})
	})
})