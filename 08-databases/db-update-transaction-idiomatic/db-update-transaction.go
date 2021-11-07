package main

import (
	"context"
	"database/sql"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"github.com/iproduct/coursego/08-databases/entities"
	"github.com/iproduct/coursego/08-databases/utils"
	"log"
	"time"
)

var (
	ctx context.Context
	db  *sql.DB
)

func main() {
	db, err := sql.Open("mysql", "root:root@/golang_projects_2021?parseTime=true")
	if err != nil {
		panic(err.Error()) // Just for example purpose. You should use proper error handling instead of panic
	}
	defer db.Close()
	db.SetConnMaxLifetime(time.Minute * 5)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)
	db.SetConnMaxIdleTime(time.Minute * 3)

	// Ping and PingContext may be used to determine if communication with
	// the database server is still possible.
	//
	// When used in a command line application Ping may be used to establish
	// that further queries are possible; that the provided DSN is valid.
	//
	// When used in long running service Ping may be part of the health
	// checking system.
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	status := "up"
	if err := db.PingContext(ctx); err != nil {
		status = "down"
	}
	log.Println(status)

	// Print projects before update
	projects := GetProjects(ctx, db)
	utils.PrintProjects(projects)

	// Update project budgets by 10% increase for project after 2020 in a single transaction
	loc, _ := time.LoadLocation("Europe/Sofia")
	const shortForm = "2006-Jan-02"
	startDate, _ := time.ParseInLocation(shortForm, "1991-Jan-01", loc)

	// BEGIN TRANSACTION
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable}) // or db.BeginTx()
	if err != nil {
		log.Fatal(err)
	}

	// Print projects after update
	projects = GetProjects(ctx, db)
	utils.PrintProjects(projects)
}

// Helper functions
// CreateOrder creates an order for an album and returns the new order ID.
func UpdateProjectBudgets(ctx context.Context, projectID, quantity, custID int) (orderID int64, err error) {

	// Get a Tx for making transaction requests.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fail(err)
	}
	// Defer a rollback in case anything fails.
	defer tx.Rollback()

	// Confirm that album inventory is enough for the order.
	var enough bool
	if err = tx.QueryRowContext(ctx, "SELECT (quantity >= ?) from album where id = ?",
		quantity, albumID).Scan(&enough); err != nil {
		if err == sql.ErrNoRows {
			return fail(fmt.Errorf("no such album"))
		}
		return fail(err)
	}
	if !enough {
		return fail(fmt.Errorf("not enough inventory"))
	}

	// Create a helper function for preparing failure results.
	fail := func(err error) (int64, error) {
		return fmt.Errorf("CreateOrder: %v", err)
	}

	result, execErr := tx.ExecContext(ctx, `UPDATE projects SET budget = ROUND(budget * 1.2) WHERE id = ?;`, projectID)
	if execErr != nil { // ROLLBSACK IF ERROR
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			log.Fatalf("update failed: %v, unable to rollback: %v\n", execErr, rollbackErr)
		}
		log.Fatalf("update failed: %v", execErr)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Total budgets updated: %d\n", rows)

	// COMMIT TRANSACTION
	if err := tx.Commit(); err != nil {
		log.Fatal(err)
	}






	// Update the album inventory to remove the quantity in the order.
	_, err = tx.ExecContext(ctx, "UPDATE album SET quantity = quantity - ? WHERE id = ?",
		quantity, albumID)
	if err != nil {
		return fail(err)
	}

	// Create a new row in the album_order table.
	result, err := tx.ExecContext(ctx, "INSERT INTO album_order (album_id, cust_id, quantity, date) VALUES (?, ?, ?, ?)",
		albumID, custID, quantity, time.Now())
	if err != nil {
		return fail(err)
	}
	// Get the ID of the order item just created.
	orderID, err := result.LastInsertId()
	if err != nil {
		return fail(err)
	}

	// Commit the transaction.
	if err = tx.Commit(); err != nil {
		return fail(err)
	}

	// Return the order ID.
	return orderID, nil
}

func fail(err error) {
	log.Panic(err)
}
func GetProjects(ctx context.Context, conn *sql.DB) []entities.Project {
	rows, err := conn.QueryContext(ctx, "SELECT * FROM projects")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	projects := []entities.Project{}
	for rows.Next() {
		p := entities.Project{}
		if err := rows.Scan(&p.Id, &p.Name, &p.Description, &p.Budget, &p.Finished, &p.StartDate, &p.CompanyId); err != nil {
			log.Fatal(err)
		}
		projects = append(projects, p)
	}
	err = rows.Close()
	if err != nil {
		log.Fatal(err)
	}
	if err = rows.Err(); err != nil {
		log.Fatal(err)
	}

	for i := range projects {
		userRows, err := conn.QueryContext(ctx, "SELECT user_id FROM projects_users WHERE project_id = ?", projects[i].Id)
		if err != nil {
			log.Fatal(err)
		}
		var userId int64
		for userRows.Next() {
			if err := userRows.Scan(&userId); err != nil {
				log.Fatal(err)
			}
			projects[i].UserIds = append(projects[i].UserIds, userId)
		}
		err = userRows.Close()
		if err != nil {
			log.Fatal(err)
		}
		if err = userRows.Err(); err != nil {
			log.Fatal(err)
		}
	}

	return projects
}
