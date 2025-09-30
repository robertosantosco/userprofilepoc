package graph_test

import (
	"context"
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"userprofilepoc/src/domain"
	"userprofilepoc/src/domain/entities"
	"userprofilepoc/src/helper/env"
	"userprofilepoc/src/infra/postgres"
	"userprofilepoc/src/infra/redis"
	"userprofilepoc/src/repositories"
	"userprofilepoc/src/services/graph"
	"userprofilepoc/src/test_artefacts/comparer"
	"userprofilepoc/src/test_artefacts/stubs"
	"userprofilepoc/src/test_artefacts/test_seeder"
)

var _ = Describe("SyncGraph", func() {
	var (
		readWriteClient       *postgres.ReadWriteClient
		testSeeder            test_seeder.TestSeeder
		graphService          *graph.GraphService
		cachedGraphRepository *repositories.CachedGraphRepository
		graphQueryRepository  *repositories.GraphQueryRepository
		graphWriteRepository  *repositories.GraphWriteRepository
		redisClient           *redis.RedisClient
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

	// Redis config (opcional para testes)
	redisAddrs := env.GetString("TEST_REDIS_HOSTS", "")
	redisPoolSize := env.GetInt("TEST_REDIS_POOL_SIZE", 10)
	redisTTL := env.GetInt("TEST_REDIS_TTL_SECONDS", 1)

	BeforeEach(func() {
		ctx = context.Background()

		// Conexão com o banco de teste
		readWriteClient, err = postgres.NewReadWriteClient(dbReadHost, dbWriteHost, dbReadPort, dbWritePort, dbname, dbUser, dbPassword, maxConnections)
		if err != nil {
			panic(err)
		}

		redisClient = redis.NewRedisClient(redisAddrs, redisPoolSize, time.Duration(redisTTL)*time.Second).WithPrefix("test:")

		// Setup dos componentes
		graphQueryRepository = repositories.NewGraphQueryRepository(readWriteClient.GetReadPool())
		cachedGraphRepository = repositories.NewCachedGraphRepository(graphQueryRepository, redisClient)
		graphWriteRepository = repositories.NewGraphWriteRepository(readWriteClient.GetWritePool(), cachedGraphRepository)
		graphService = graph.NewGraphService(cachedGraphRepository, graphWriteRepository)
		testSeeder = test_seeder.New(readWriteClient.GetWritePool())

		// Limpar dados
		testSeeder.TruncateTables(ctx)
		redisClient.FlushByPrefix(ctx)
	})

	AfterEach(func() {
		if readWriteClient.GetReadPool() != nil {
			readWriteClient.GetReadPool().Close()
		}

		if readWriteClient.GetWritePool() != nil {
			readWriteClient.GetWritePool().Close()
		}
	})

	Context("when syncing with empty request", func() {
		When("calling SyncGraph with empty entities and relationships", func() {
			It("returns an error about empty request", func() {
				// ACT
				err := graphService.SyncGraph(ctx, domain.SyncGraphRequest{
					Entities:      []domain.SyncEntityDTO{},
					Relationships: []domain.SyncRelationshipDTO{},
				})

				// ASSERT
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("sync request must contain at least one entity or relationship"))
			})
		})
	})

	Context("when syncing new entities", func() {
		When("calling SyncGraph with new entities only", func() {
			It("creates the entities successfully", func() {
				// ARRANGE
				rootEntity1 := stubs.NewEntityStub().WithType("user").Get()
				rootEntity2 := stubs.NewEntityStub().WithType("account").Get()

				request := domain.SyncGraphRequest{
					Entities: []domain.SyncEntityDTO{
						{
							Reference:  rootEntity1.Reference,
							Type:       rootEntity1.Type,
							Properties: rootEntity1.Properties,
						},
						{
							Reference:  rootEntity2.Reference,
							Type:       rootEntity2.Type,
							Properties: rootEntity2.Properties,
						},
					},
					Relationships: []domain.SyncRelationshipDTO{},
				}

				expected := []entities.Entity{
					rootEntity1,
					rootEntity2,
				}

				// ACT
				err := graphService.SyncGraph(ctx, request)

				// ASSERT
				Expect(err).NotTo(HaveOccurred())

				// Verify entities were created
				databaseEntities, err := testSeeder.SelectEntitiesByReferences(ctx, []string{rootEntity1.Reference, rootEntity2.Reference})
				Expect(err).NotTo(HaveOccurred())

				Expect(databaseEntities).To(HaveLen(2))
				Expect(databaseEntities).To(ContainElements(
					BeComparableTo(expected[0], comparer.TimeWithinTolerance(200), comparer.JSONRawMessage(), comparer.IgnoreFieldsFor[entities.Entity]("ID")),
					BeComparableTo(expected[1], comparer.TimeWithinTolerance(200), comparer.JSONRawMessage(), comparer.IgnoreFieldsFor[entities.Entity]("ID")),
				))
			})
		})
	})

	Context("when syncing existing entities with new properties", func() {

		When("calling SyncGraph with entities that already exist", func() {
			It("merges properties correctly", func() {
				// ARRANGE
				existingEntity := stubs.NewEntityStub().
					WithType("user").
					WithProperties(map[string]interface{}{
						"name": "John Doe",
						"age":  30,
					}).
					Get()
				testSeeder.InsertEntity(ctx, &existingEntity)

				newProperties := map[string]interface{}{
					"email": "john@example.com",
					"age":   31,
				}
				newPropsJSON, _ := json.Marshal(newProperties)

				request := domain.SyncGraphRequest{
					Entities: []domain.SyncEntityDTO{
						{
							Reference:  existingEntity.Reference,
							Type:       existingEntity.Type,
							Properties: newPropsJSON,
						},
					},
					Relationships: []domain.SyncRelationshipDTO{},
				}

				mergedProps := map[string]interface{}{
					"name":  "John Doe",
					"age":   31,
					"email": "john@example.com",
				}
				mergedPropsJSON, _ := json.Marshal(mergedProps)

				expected := entities.Entity{
					Type:       existingEntity.Type,
					Reference:  existingEntity.Reference,
					Properties: mergedPropsJSON,
					CreatedAt:  existingEntity.CreatedAt,
					UpdatedAt:  existingEntity.UpdatedAt,
				}

				// ACT
				err := graphService.SyncGraph(ctx, request)

				// ASSERT
				Expect(err).NotTo(HaveOccurred())
				databaseEntities, err := testSeeder.SelectEntitiesByReferences(ctx, []string{existingEntity.Reference})
				Expect(err).NotTo(HaveOccurred())
				Expect(databaseEntities).To(HaveLen(1))
				Expect(databaseEntities).To(ContainElements(
					BeComparableTo(expected, comparer.TimeWithinTolerance(200), comparer.JSONRawMessage(), comparer.IgnoreFieldsFor[entities.Entity]("ID")),
				))
			})
		})

		When("calling SyncGraph with partial property updates", func() {
			It("updates only specified fields while preserving others", func() {
				// ARRANGE
				existingEntity := stubs.NewEntityStub().
					WithType("user").
					WithProperties(map[string]interface{}{
						"name":   "John Doe",
						"age":    30,
						"city":   "New York",
						"active": true,
					}).
					Get()
				testSeeder.InsertEntity(ctx, &existingEntity)

				partialUpdate := map[string]interface{}{
					"age":  31,
					"city": "San Francisco",
				}
				partialUpdateJSON, _ := json.Marshal(partialUpdate)

				request := domain.SyncGraphRequest{
					Entities: []domain.SyncEntityDTO{
						{
							Reference:  existingEntity.Reference,
							Type:       existingEntity.Type,
							Properties: partialUpdateJSON,
						},
					},
					Relationships: []domain.SyncRelationshipDTO{},
				}

				mergedProps := map[string]interface{}{
					"name":   "John Doe",
					"age":    31,
					"city":   "San Francisco",
					"active": true,
				}
				mergedPropsJSON, _ := json.Marshal(mergedProps)

				expected := entities.Entity{
					Type:       existingEntity.Type,
					Reference:  existingEntity.Reference,
					Properties: mergedPropsJSON,
					CreatedAt:  existingEntity.CreatedAt,
					UpdatedAt:  existingEntity.UpdatedAt,
				}

				// ACT
				err := graphService.SyncGraph(ctx, request)

				// ASSERT
				databaseEntities, err := testSeeder.SelectEntitiesByReferences(ctx, []string{existingEntity.Reference})
				Expect(err).NotTo(HaveOccurred())
				Expect(databaseEntities).To(HaveLen(1))
				Expect(databaseEntities).To(ContainElements(
					BeComparableTo(expected, comparer.TimeWithinTolerance(200), comparer.JSONRawMessage(), comparer.IgnoreFieldsFor[entities.Entity]("ID")),
				))
			})
		})
	})

	Context("when syncing new relationships", func() {
		When("calling SyncGraph with relationships between existing entities", func() {
			It("creates the relationships successfully", func() {
				// ARRANGE
				rootEntity := stubs.NewEntityStub().WithType("root").Get()
				childEntity := stubs.NewEntityStub().WithType("child").Get()

				testSeeder.InsertEntity(ctx, &rootEntity)
				testSeeder.InsertEntity(ctx, &childEntity)

				request := domain.SyncGraphRequest{
					Entities: []domain.SyncEntityDTO{},
					Relationships: []domain.SyncRelationshipDTO{
						{
							SourceReference:  rootEntity.Reference,
							TargetReference:  childEntity.Reference,
							RelationshipType: "has_child",
						},
					},
				}

				expected := entities.Edge{
					LeftEntityID:     rootEntity.ID,
					RightEntityID:    childEntity.ID,
					RelationshipType: "has_child",
					CreatedAt:        rootEntity.CreatedAt,
					UpdatedAt:        rootEntity.UpdatedAt,
				}

				// ACT
				err := graphService.SyncGraph(ctx, request)

				// ASSERT
				Expect(err).NotTo(HaveOccurred())
				edges, err := testSeeder.SelectEdgesByEntityReferences(ctx, []string{rootEntity.Reference})
				Expect(err).NotTo(HaveOccurred())
				Expect(edges).To(HaveLen(1))
				Expect(edges[0]).To(BeComparableTo(expected, comparer.TimeWithinTolerance(200), comparer.JSONRawMessage(), comparer.IgnoreFieldsFor[entities.Edge]("ID")))
			})
		})

		When("calling SyncGraph with relationships to non-existent entities", func() {
			It("creates missing entities and relationships", func() {
				// ARRANGE
				existingRoot := stubs.NewEntityStub().WithType("root").Get()
				testSeeder.InsertEntity(ctx, &existingRoot)

				newChild := stubs.NewEntityStub().WithType("child").Get()

				request := domain.SyncGraphRequest{
					Entities: []domain.SyncEntityDTO{
						{
							Reference:  newChild.Reference,
							Type:       newChild.Type,
							Properties: newChild.Properties,
						},
					},
					Relationships: []domain.SyncRelationshipDTO{
						{
							SourceReference:  existingRoot.Reference,
							TargetReference:  newChild.Reference,
							RelationshipType: "has_child",
						},
					},
				}

				// ACT
				err := graphService.SyncGraph(ctx, request)

				// ASSERT
				Expect(err).NotTo(HaveOccurred())

				// Verify new entity was created
				databaseEntities, err := testSeeder.SelectEntitiesByReferences(ctx, []string{newChild.Reference})
				Expect(err).NotTo(HaveOccurred())
				Expect(databaseEntities).To(HaveLen(1))

				Expect(databaseEntities[0]).To(BeComparableTo(newChild, comparer.TimeWithinTolerance(200), comparer.JSONRawMessage(), comparer.IgnoreFieldsFor[entities.Entity]("ID")))

				// Verify relationship was created
				edges, err := testSeeder.SelectEdgesByEntityReferences(ctx, []string{existingRoot.Reference})
				Expect(err).NotTo(HaveOccurred())
				Expect(edges).To(HaveLen(1))
				Expect(edges[0].LeftEntityID).To(Equal(existingRoot.ID))
				Expect(edges[0].RightEntityID).To(Equal(databaseEntities[0].ID))
				Expect(edges[0].RelationshipType).To(Equal("has_child"))
			})
		})
	})

	Context("when syncing existing relationships", func() {
		When("calling SyncGraph with relationships that already exist", func() {
			It("updates existing relationships", func() {
				// ARRANGE
				rootEntity := stubs.NewEntityStub().WithType("root").Get()
				childEntity := stubs.NewEntityStub().WithType("child").Get()

				testSeeder.InsertEntity(ctx, &rootEntity)
				testSeeder.InsertEntity(ctx, &childEntity)

				existingEdge := stubs.NewEdgeStub().
					WithLeftEntityID(rootEntity.ID).
					WithRightEntityID(childEntity.ID).
					WithRelationshipType("old_relation").
					Get()
				testSeeder.InsertEdge(ctx, &existingEdge)

				request := domain.SyncGraphRequest{
					Entities: []domain.SyncEntityDTO{},
					Relationships: []domain.SyncRelationshipDTO{
						{
							SourceReference:  rootEntity.Reference,
							TargetReference:  childEntity.Reference,
							RelationshipType: "new_relation",
						},
					},
				}

				// ACT
				err := graphService.SyncGraph(ctx, request)

				// ASSERT
				Expect(err).NotTo(HaveOccurred())

				// Verify relationship was updated
				edges, err := testSeeder.SelectEdgesByEntityReferences(ctx, []string{rootEntity.Reference})
				Expect(err).NotTo(HaveOccurred())
				Expect(edges).To(HaveLen(1))
				Expect(edges[0].RelationshipType).To(Equal("new_relation"))
			})
		})
	})

	Context("when syncing complex graph with entities and relationships", func() {

		When("calling SyncGraph with mixed entities and relationships", func() {
			It("handles all operations correctly", func() {
				// ARRANGE
				// Existing root entity that will be updated
				existingRoot := stubs.NewEntityStub().
					WithType("root").
					WithProperties(map[string]interface{}{
						"name": "Existing Root",
						"version": 1,
					}).
					Get()
				testSeeder.InsertEntity(ctx, &existingRoot)

				// New entities that will be created
				childEntity1 := stubs.NewEntityStub().WithType("child").Get()
				childEntity2 := stubs.NewEntityStub().WithType("child").Get()

				// Properties to update existing root
				rootUpdate := map[string]interface{}{
					"status": "active",
					"version": 2,
				}
				rootUpdateJSON, _ := json.Marshal(rootUpdate)

				request := domain.SyncGraphRequest{
					Entities: []domain.SyncEntityDTO{
						{
							Reference:  existingRoot.Reference, // existing - should merge properties
							Type:       existingRoot.Type,
							Properties: rootUpdateJSON,
						},
						{
							Reference:  childEntity1.Reference, // new entity
							Type:       childEntity1.Type,
							Properties: childEntity1.Properties,
						},
						{
							Reference:  childEntity2.Reference, // new entity
							Type:       childEntity2.Type,
							Properties: childEntity2.Properties,
						},
					},
					Relationships: []domain.SyncRelationshipDTO{
						{
							SourceReference:  existingRoot.Reference,
							TargetReference:  childEntity1.Reference,
							RelationshipType: "has_child",
						},
						{
							SourceReference:  existingRoot.Reference,
							TargetReference:  childEntity2.Reference,
							RelationshipType: "has_child",
						},
						{
							SourceReference:  childEntity1.Reference,
							TargetReference:  childEntity2.Reference,
							RelationshipType: "sibling_of",
						},
					},
				}

				// Expected merged properties for root
				mergedRootProps := map[string]interface{}{
					"name":    "Existing Root",
					"version": 2,
					"status":  "active",
				}
				mergedRootPropsJSON, _ := json.Marshal(mergedRootProps)

				expectedEntities := []entities.Entity{
					{
						Type:       existingRoot.Type,
						Reference:  existingRoot.Reference,
						Properties: mergedRootPropsJSON,
						CreatedAt:  existingRoot.CreatedAt,
						UpdatedAt:  existingRoot.UpdatedAt,
					},
					childEntity1,
					childEntity2,
				}

				expectedRelationships := []struct {
					SourceReference  string
					TargetReference  string
					RelationshipType string
				}{
					{existingRoot.Reference, childEntity1.Reference, "has_child"},
					{existingRoot.Reference, childEntity2.Reference, "has_child"},
					{childEntity1.Reference, childEntity2.Reference, "sibling_of"},
				}

				// ACT
				err := graphService.SyncGraph(ctx, request)

				// ASSERT
				Expect(err).NotTo(HaveOccurred())

				databaseEntities, err := testSeeder.SelectEntitiesByReferences(ctx, []string{existingRoot.Reference, childEntity1.Reference, childEntity2.Reference})
				Expect(err).NotTo(HaveOccurred())
				Expect(databaseEntities).To(HaveLen(3))

				Expect(databaseEntities).To(ContainElements(
					BeComparableTo(expectedEntities[0], comparer.TimeWithinTolerance(200), comparer.JSONRawMessage(), comparer.IgnoreFieldsFor[entities.Entity]("ID")),
					BeComparableTo(expectedEntities[1], comparer.TimeWithinTolerance(200), comparer.JSONRawMessage(), comparer.IgnoreFieldsFor[entities.Entity]("ID")),
					BeComparableTo(expectedEntities[2], comparer.TimeWithinTolerance(200), comparer.JSONRawMessage(), comparer.IgnoreFieldsFor[entities.Entity]("ID")),
				))

				// Build entity ID map for relationship verification
				entityIDMap := make(map[string]int64)
				for i := range databaseEntities {
					entityIDMap[databaseEntities[i].Reference] = databaseEntities[i].ID
				}

				// Verify relationships
				databaseEdges, err := testSeeder.SelectEdgesByEntityReferences(ctx, []string{existingRoot.Reference, childEntity1.Reference, childEntity2.Reference})
				Expect(err).NotTo(HaveOccurred())
				Expect(databaseEdges).To(HaveLen(3))

				// Build expected edges with actual IDs from database
				expectedEdges := make([]entities.Edge, len(expectedRelationships))
				for i, rel := range expectedRelationships {
					expectedEdges[i] = entities.Edge{
						LeftEntityID:     entityIDMap[rel.SourceReference],
						RightEntityID:    entityIDMap[rel.TargetReference],
						RelationshipType: rel.RelationshipType,
						CreatedAt:        existingRoot.CreatedAt,
						UpdatedAt:        existingRoot.UpdatedAt,
					}
				}

				Expect(databaseEdges).To(ContainElements(
					BeComparableTo(expectedEdges[0], comparer.TimeWithinTolerance(200), comparer.JSONRawMessage(), comparer.IgnoreFieldsFor[entities.Edge]("ID")),
					BeComparableTo(expectedEdges[1], comparer.TimeWithinTolerance(200), comparer.JSONRawMessage(), comparer.IgnoreFieldsFor[entities.Edge]("ID")),
					BeComparableTo(expectedEdges[2], comparer.TimeWithinTolerance(200), comparer.JSONRawMessage(), comparer.IgnoreFieldsFor[entities.Edge]("ID")),
				))
			})
		})
	})

	Context("when syncing with null properties", func() {

		When("calling SyncGraph with entity having null properties", func() {
			It("handles null properties correctly", func() {
				// ARRANGE
				existingEntity := stubs.NewEntityStub().WithType("root").Get()
				testSeeder.InsertEntity(ctx, &existingEntity)

				request := domain.SyncGraphRequest{
					Entities: []domain.SyncEntityDTO{
						{
							Reference:  existingEntity.Reference,
							Type:       existingEntity.Type,
							Properties: nil,
						},
					},
					Relationships: []domain.SyncRelationshipDTO{},
				}

				expected := entities.Entity{
					Type:       existingEntity.Type,
					Reference:  existingEntity.Reference,
					Properties: nil,
					CreatedAt:  existingEntity.CreatedAt,
					UpdatedAt:  existingEntity.UpdatedAt,
				}

				// ACT
				err := graphService.SyncGraph(ctx, request)

				// ASSERT
				Expect(err).NotTo(HaveOccurred())

				databaseEntities, err := testSeeder.SelectEntitiesByReferences(ctx, []string{existingEntity.Reference})
				Expect(err).NotTo(HaveOccurred())
				Expect(databaseEntities).To(HaveLen(1))

				Expect(databaseEntities).To(ContainElements(
					BeComparableTo(expected, comparer.TimeWithinTolerance(200), comparer.JSONRawMessage(), comparer.IgnoreFieldsFor[entities.Entity]("ID")),
				))
			})
		})

		When("calling SyncGraph creating new entity with null properties", func() {
			It("creates entity with empty JSON properties", func() {
				// ARRANGE
				newEntity := stubs.NewEntityStub().WithType("root").Get()

				request := domain.SyncGraphRequest{
					Entities: []domain.SyncEntityDTO{
						{
							Reference:  newEntity.Reference,
							Type:       newEntity.Type,
							Properties: nil,
						},
					},
					Relationships: []domain.SyncRelationshipDTO{},
				}

				expected := entities.Entity{
					Type:       newEntity.Type,
					Reference:  newEntity.Reference,
					Properties: nil,
					CreatedAt:  newEntity.CreatedAt,
					UpdatedAt:  newEntity.UpdatedAt,
				}

				// ACT
				err := graphService.SyncGraph(ctx, request)

				// ASSERT
				Expect(err).NotTo(HaveOccurred())

				databaseEntities, err := testSeeder.SelectEntitiesByReferences(ctx, []string{newEntity.Reference})
				Expect(err).NotTo(HaveOccurred())
				Expect(databaseEntities).To(HaveLen(1))

				Expect(databaseEntities).To(ContainElements(
					BeComparableTo(expected, comparer.TimeWithinTolerance(200), comparer.JSONRawMessage(), comparer.IgnoreFieldsFor[entities.Entity]("ID")),
				))
			})
		})
	})

	Context("when syncing duplicate entities in same request", func() {

		When("calling SyncGraph with duplicate entity references", func() {
			It("processes entities correctly by merging properties", func() {
				// ARRANGE
				duplicateEntity := stubs.NewEntityStub().WithType("root").Get()

				props1 := map[string]interface{}{
					"name":    "Initial Name",
					"version": 1,
				}
				props1JSON, _ := json.Marshal(props1)

				props2 := map[string]interface{}{
					"name": "Updated Name",
					"age":  30,
				}
				props2JSON, _ := json.Marshal(props2)

				request := domain.SyncGraphRequest{
					Entities: []domain.SyncEntityDTO{
						{
							Reference:  duplicateEntity.Reference,
							Type:       duplicateEntity.Type,
							Properties: props1JSON,
						},
						{
							Reference:  duplicateEntity.Reference,
							Type:       duplicateEntity.Type,
							Properties: props2JSON,
						},
					},
					Relationships: []domain.SyncRelationshipDTO{},
				}

				mergedProps := map[string]interface{}{
					"name":    "Updated Name",
					"age":     30,
					"version": 1,
				}
				mergedPropsJSON, _ := json.Marshal(mergedProps)

				expected := entities.Entity{
					Type:       duplicateEntity.Type,
					Reference:  duplicateEntity.Reference,
					Properties: mergedPropsJSON,
					CreatedAt:  duplicateEntity.CreatedAt,
					UpdatedAt:  duplicateEntity.UpdatedAt,
				}

				// ACT
				err := graphService.SyncGraph(ctx, request)

				// ASSERT
				Expect(err).NotTo(HaveOccurred())

				databaseEntities, err := testSeeder.SelectEntitiesByReferences(ctx, []string{duplicateEntity.Reference})
				Expect(err).NotTo(HaveOccurred())
				Expect(databaseEntities).To(HaveLen(1))

				Expect(databaseEntities).To(ContainElements(
					BeComparableTo(expected, comparer.TimeWithinTolerance(200), comparer.JSONRawMessage(), comparer.IgnoreFieldsFor[entities.Entity]("ID")),
				))
			})
		})
	})
})
