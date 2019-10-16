package sqlgen

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

type limitTestcase struct {
	title           string
	withFilterLimit func(*DB, Filter) *DB
}

func getLimitTestcases(t *testing.T, operation string) []limitTestcase {
	return []limitTestcase{
		{
			title: fmt.Sprintf("%s with shard limit", operation),
			withFilterLimit: func(db *DB, filter Filter) *DB {
				db, err := db.WithShardLimit(filter)
				assert.NoError(t, err)
				return db
			},
		},
		{
			title: fmt.Sprintf("%s with dynamic limit", operation),
			withFilterLimit: func(db *DB, filter Filter) *DB {
				dynamicLimitFunc := func(ctx context.Context, table string) Filter {
					// We can be generic here, not testing dynamic limits specifically.
					assert.Contains(t, []string{"users", "just_ids"}, table)
					return filter
				}
				shouldKeepGoingOnErrorFunc := func(err error, table string) bool {
					// We can be generic here, not testing dynamic limits specifically.
					assert.Contains(t, []string{"users", "just_ids"}, table)
					return false
				}
				db, _ = db.WithDynamicLimit(
					DynamicLimit{
						dynamicLimitFunc,
						shouldKeepGoingOnErrorFunc,
					},
				)
				return db
			},
		},
	}
}

func TestShardLimitTwice(t *testing.T) {
	tdb, db, err := setup()
	assert.NoError(t, err)
	defer tdb.Close()
	aliceDb, err := db.WithShardLimit(Filter{
		"name": "Alice",
	})
	assert.NoError(t, err)

	_, err = aliceDb.WithShardLimit(nil)
	assert.EqualError(t, err, "already has shard limit")
}

func TestDynamicLimitTwice(t *testing.T) {
	tdb, db, err := setup()
	assert.NoError(t, err)

	dynamicLimitFunc := func(ctx context.Context, table string) Filter {
		return Filter{
			"name": "Alice",
		}
	}
	dynamicLimitErrorFunc := func(err error, table string) bool {
		return false
	}

	defer tdb.Close()
	aliceDb, err := db.WithDynamicLimit(
		DynamicLimit{
			dynamicLimitFunc,
			dynamicLimitErrorFunc,
		},
	)
	assert.NoError(t, err)

	_, err = aliceDb.WithDynamicLimit(DynamicLimit{})
	assert.EqualError(t, err, "already has dynamic limit")
}

func TestShardLimitInsert(t *testing.T) {
	testcases := getLimitTestcases(t, "Insert")
	for _, testcase := range testcases {
		t.Run(testcase.title, func(t *testing.T) {
			tdb, db, err := setup()
			assert.NoError(t, err)
			defer tdb.Close()
			ctx := context.Background()

			aliceDb := testcase.withFilterLimit(db, Filter{
				"name": "Alice",
			})

			alice := &User{Name: "Alice"}
			bob := &User{Name: "Bob"}

			// Check aliceDb can insert alice.
			res, err := aliceDb.InsertRow(ctx, alice)
			assert.NoError(t, err)

			id, err := res.LastInsertId()
			assert.NoError(t, err)
			alice.Id = id

			// Check aliceDb can't insert bob.
			_, err = aliceDb.InsertRow(ctx, bob)
			assert.Contains(t, err.Error(), "db requies name = Alice, but query has name = Bob")

			// Check db can still insert bob.
			_, err = db.InsertRow(ctx, bob)
			assert.NoError(t, err)
		})
	}
}

func TestShardLimitQueryRow(t *testing.T) {
	testcases := getLimitTestcases(t, "Query row")
	for _, testcase := range testcases {
		t.Run(testcase.title, func(t *testing.T) {
			tdb, db, err := setup()
			assert.NoError(t, err)

			defer tdb.Close()
			ctx := context.Background()

			aliceDb, _ := db.WithShardLimit(Filter{
				"name": "Alice",
			})

			alice := &User{Name: "Alice"}
			aliceDb.InsertRow(ctx, alice)

			// Check aliceDb can query alice.
			var user *User
			err = aliceDb.QueryRow(ctx, &user, Filter{"name": "Alice"}, nil)
			assert.NoError(t, err)

			// Check aliceDb can't query bob.
			err = aliceDb.QueryRow(ctx, &user, Filter{"name": "Bob"}, nil)
			assert.Contains(t, err.Error(), "db requires name = Alice, but query specifies name = Bob")
		})
	}
}

func TestShardLimitCount(t *testing.T) {
	testcases := getLimitTestcases(t, "Count")
	for _, testcase := range testcases {
		t.Run(testcase.title, func(t *testing.T) {
			tdb, db, err := setup()
			assert.NoError(t, err)

			defer tdb.Close()
			ctx := context.Background()

			aliceDb := testcase.withFilterLimit(db, Filter{
				"name": "Alice",
			})

			alice := &User{Name: "Alice"}
			aliceDb.InsertRow(ctx, alice)

			// Check aliceDb can count alice.
			_, err = aliceDb.Count(ctx, &User{}, Filter{"name": "Alice"})
			assert.NoError(t, err)

			// Check aliceDb can't count bob.
			_, err = aliceDb.Count(ctx, &User{}, Filter{"name": "Bob"})
			assert.Contains(t, err.Error(), "db requires name = Alice, but query specifies name = Bob")

			// Check aliceDb can't count everything.
			_, err = aliceDb.Count(ctx, &User{}, nil)
			assert.Contains(t, err.Error(), "db requires name = Alice, but query does not filter on name")
		})
	}
}

func TestUpdateWithLimit(t *testing.T) {
	testcases := getLimitTestcases(t, "Update")
	for _, testcase := range testcases {
		t.Run(testcase.title, func(t *testing.T) {
			tdb, db, err := setup()
			assert.NoError(t, err)

			defer tdb.Close()
			ctx := context.Background()

			aliceDb := testcase.withFilterLimit(db, Filter{
				"name": "Alice",
			})

			alice := &User{Name: "Alice"}
			bob := &User{Name: "Bob"}

			aliceDb.InsertRow(ctx, alice)
			db.InsertRow(ctx, bob)

			// Check aliceDb can update alice.
			err = aliceDb.UpdateRow(ctx, alice)
			assert.NoError(t, err)

			// Check aliceDb can't update bob.
			err = aliceDb.UpdateRow(ctx, bob)
			assert.Contains(t, err.Error(), "name = Alice")

			// Check old db can update bob
			err = db.UpdateRow(ctx, bob)
			assert.NoError(t, err)
		})
	}
}

func TestDeleteWithLimit(t *testing.T) {
	testcases := getLimitTestcases(t, "Delete")
	for _, testcase := range testcases {
		t.Run(testcase.title, func(t *testing.T) {
			tdb, db, err := setup()
			assert.NoError(t, err)
			defer tdb.Close()
			ctx := context.Background()

			aliceDb := testcase.withFilterLimit(db, Filter{
				"name": "Alice",
			})

			alice := &User{Name: "Alice"}
			bob := &User{Name: "Bob"}
			res, _ := aliceDb.InsertRow(ctx, alice)
			id, _ := res.LastInsertId()
			alice.Id = id
			db.InsertRow(ctx, bob)

			// Deletes only include the primary key in the filter. This means
			// we need to limit to the exact ID to do deletes. Check that.
			aliceIdDb := testcase.withFilterLimit(db, Filter{
				"id": alice.Id,
			})

			// Check aliceDb can't delete alice.
			err = aliceDb.DeleteRow(ctx, alice)
			assert.Contains(t, err.Error(), "name = Alice")

			// Check aliceIdDb can delete alice.
			err = aliceIdDb.DeleteRow(ctx, alice)
			assert.NoError(t, err)

			// Check aliceIdDb can't delete bob.
			err = aliceIdDb.DeleteRow(ctx, bob)
			assert.Contains(t, err.Error(), fmt.Sprintf("id = %d", alice.Id))
		})
	}
}

func TestUpsertWithLimit(t *testing.T) {
	testcases := getLimitTestcases(t, "Upsert")
	for _, testcase := range testcases {
		t.Run(testcase.title, func(t *testing.T) {
			tdb, db, err := setup()
			assert.NoError(t, err)
			defer tdb.Close()
			ctx := context.Background()

			// To test upsert we must have a unique primary key.
			id1 := &JustId{Id: 1}
			id2 := &JustId{Id: 2}

			just1Db := testcase.withFilterLimit(db, Filter{"id": int64(1)})

			// Check just1Db can upsert id1.
			_, err = just1Db.UpsertRow(ctx, id1)
			assert.NoError(t, err)

			// Check just1Db can't upsert id2.
			_, err = just1Db.UpsertRow(ctx, id2)
			assert.Contains(t, err.Error(), "id = 1")
		})
	}
}

func TestDynamicFilterErrorFuncBasic(t *testing.T) {

	testcases := []struct {
		title                      string
		shouldKeepGoingOnErrorFunc DynamicLimitErrorCallback
		expectSoftFail             bool
	}{
		{
			title: "Error func soft fails",
			shouldKeepGoingOnErrorFunc: func(err error, table string) bool {
				return true
			},
			expectSoftFail: true,
		},
		{
			title: "Error func hard fails",
			shouldKeepGoingOnErrorFunc: func(err error, table string) bool {
				return false
			},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.title, func(t *testing.T) {
			tdb, db, err := setup()
			assert.NoError(t, err)

			defer tdb.Close()
			ctx := context.Background()

			dynamicLimitFuncName := func(ctx context.Context, table string) Filter {
				assert.Equal(t, "users", table)
				return Filter{
					"name": "Alice",
				}
			}

			aliceDb, _ := db.WithDynamicLimit(DynamicLimit{
				dynamicLimitFuncName,
				testcase.shouldKeepGoingOnErrorFunc,
			})

			alice := &User{Name: "Alice"}
			bob := &User{Name: "Bob"}
			res, _ := aliceDb.InsertRow(ctx, alice)
			id, _ := res.LastInsertId()
			alice.Id = id
			db.InsertRow(ctx, bob)

			dynamicLimitFuncID := func(ctx context.Context, table string) Filter {
				assert.Equal(t, "users", table)
				return Filter{
					"id": alice.Id,
				}
			}

			// Deletes only include the primary key in the filter. This means
			// we need to limit to the exact ID to do deletes. Check that.
			aliceIDDb, _ := db.WithDynamicLimit(DynamicLimit{
				dynamicLimitFuncID,
				testcase.shouldKeepGoingOnErrorFunc,
			})

			// aliceDb normally can't delete alice with the limit, this should return an error
			// on hard fail, but no error on soft fail.
			err = aliceDb.DeleteRow(ctx, alice)
			if testcase.expectSoftFail {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}

			// aliceIdDb normally can delete alice.
			err = aliceIDDb.DeleteRow(ctx, alice)
			assert.NoError(t, err)

			// aliceIdDb normally can't delete bob with the limit, this should return an error
			// on hard fail, but no error on soft fail.
			err = aliceIDDb.DeleteRow(ctx, bob)
			if testcase.expectSoftFail {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}

		})
	}
}

func TestDynamicFilterErrorFuncDetailederror(t *testing.T) {

	testcases := []struct {
		title                      string
		shouldKeepGoingOnErrorFunc DynamicLimitErrorCallback
		expectSoftFail             bool
	}{
		{
			title: "Error func soft fails",
			shouldKeepGoingOnErrorFunc: func(err error, table string) bool {
				assert.Contains(t, err.Error(), "query clause: 'DELETE FROM users WHERE id = ?';")
				return true
			},
			expectSoftFail: true,
		},
		{
			title: "Error func hard fails",
			shouldKeepGoingOnErrorFunc: func(err error, table string) bool {
				assert.Contains(t, err.Error(), "query clause: 'DELETE FROM users WHERE id = ?';")
				assert.Condition(
					t,
					func() bool {
						return strings.Contains(err.Error(), "; query args: '[0]'") || strings.Contains(err.Error(), "; query args: '[1]'")
					},
				)
				return false
			},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.title, func(t *testing.T) {
			tdb, db, err := setup()
			assert.NoError(t, err)

			defer tdb.Close()
			ctx := context.Background()

			dynamicLimitFuncName := func(ctx context.Context, table string) Filter {
				assert.Equal(t, "users", table)
				return Filter{
					"name": "Alice",
				}
			}

			aliceDb, _ := db.WithDynamicLimit(DynamicLimit{
				dynamicLimitFuncName,
				testcase.shouldKeepGoingOnErrorFunc,
			})

			alice := &User{Name: "Alice"}
			bob := &User{Name: "Bob"}
			res, _ := aliceDb.InsertRow(ctx, alice)
			id, _ := res.LastInsertId()
			alice.Id = id
			db.InsertRow(ctx, bob)

			dynamicLimitFuncID := func(ctx context.Context, table string) Filter {
				assert.Equal(t, "users", table)
				return Filter{
					"id": alice.Id,
				}
			}

			// Deletes only include the primary key in the filter. This means
			// we need to limit to the exact ID to do deletes. Check that.
			aliceIDDb, _ := db.WithDynamicLimit(DynamicLimit{
				dynamicLimitFuncID,
				testcase.shouldKeepGoingOnErrorFunc,
			})

			// aliceDb normally can't delete alice with the limit, this should fail.
			err = aliceDb.DeleteRow(ctx, alice)

			// aliceIdDb normally can delete alice.
			err = aliceIDDb.DeleteRow(ctx, alice)
			assert.NoError(t, err)

			// aliceIdDb normally can't delete bob with the limit, this should fail.
			err = aliceIDDb.DeleteRow(ctx, bob)
		})
	}
}

func TestDynamicFilterLimitCanTellTablesApart(t *testing.T) {
	shouldKeepGoingOnErrorFunc := func(err error, table string) bool {
		return false
	}
	dynamicFilter := func(ctx context.Context, table string) Filter {
		if table == "users" {
			return Filter{
				"name": "Alice",
			}
		} else if table == "just_ids" {
			return Filter{"id": int64(1)}
		} else {
			t.Error("unexpected table in dynamic filter")
		}
		return nil
	}

	tdb, db, err := setup()
	assert.NoError(t, err)
	defer tdb.Close()
	ctx := context.Background()

	aliceDb, _ := db.WithDynamicLimit(DynamicLimit{
		dynamicFilter,
		shouldKeepGoingOnErrorFunc,
	})

	alice := &User{Name: "Alice"}
	bob := &User{Name: "Bob"}

	aliceDb.InsertRow(ctx, alice)
	db.InsertRow(ctx, bob)

	// Check aliceDb can update alice.
	err = aliceDb.UpdateRow(ctx, alice)
	assert.NoError(t, err)

	// Check aliceDb can't update bob.
	err = aliceDb.UpdateRow(ctx, bob)
	assert.Contains(t, err.Error(), "name = Alice")

	// Check old db can update bob
	err = db.UpdateRow(ctx, bob)
	assert.NoError(t, err)

	// To test upsert we must have a unique primary key.
	id1 := &JustId{Id: 1}
	id2 := &JustId{Id: 2}

	just1Db, err := db.WithDynamicLimit(DynamicLimit{
		dynamicFilter,
		shouldKeepGoingOnErrorFunc,
	})
	assert.NoError(t, err)

	// Check just1Db can upsert id1.
	_, err = just1Db.UpsertRow(ctx, id1)
	assert.NoError(t, err)

	// Check just1Db can't upsert id2.
	_, err = just1Db.UpsertRow(ctx, id2)
	assert.Contains(t, err.Error(), "id = 1")

}

func TestDynamicFilterLimitCanKeepState(t *testing.T) {
	tdb, db, err := setup()
	assert.NoError(t, err)
	defer tdb.Close()
	ctx := context.Background()

	ctr := 0
	dynamicFilterWithCtr := func(ctx context.Context, table string) Filter {
		ctr++
		return Filter{"id": int64(ctr)}
	}
	shouldKeepGoingOnErrorFunc := func(err error, table string) bool {
		return false
	}

	id1 := &JustId{Id: 1}
	id2 := &JustId{Id: 2}
	id4 := &JustId{Id: 4}

	just1Db, err := db.WithDynamicLimit(DynamicLimit{
		dynamicFilterWithCtr,
		shouldKeepGoingOnErrorFunc,
	})
	assert.NoError(t, err)

	// Check just1Db can upsert id1. (counter on filter should be 1)
	_, err = just1Db.UpsertRow(ctx, id1)
	assert.NoError(t, err)

	// Check just1Db can upsert id2 (counter on filter hould be 2)
	_, err = just1Db.UpsertRow(ctx, id2)
	assert.NoError(t, err)

	// Check just1Db can't upsert id2 (counter on filter should be 3)
	_, err = just1Db.UpsertRow(ctx, id2)
	assert.Contains(t, err.Error(), "column values check failed for db with dynamic limit: db requies id = 3, but query has id = 2")

	// Check just1Db can upsert id4 (counter on filter should be 4)
	_, err = just1Db.UpsertRow(ctx, id4)
	assert.NoError(t, err)

	// Check just1Db can't upsert id1 again (counter on filter should be 5)
	_, err = just1Db.UpsertRow(ctx, id1)
	assert.Contains(t, err.Error(), "column values check failed for db with dynamic limit: db requies id = 5, but query has id = 1")
}
