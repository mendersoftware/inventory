package wrapper

import (
	"go.mongodb.org/mongo-driver/mongo"
)

func MongoGetCollection(client *mongo.Client, dbName string, collectionName string) mongo.Collection {
	return client.Database(dbName).Collection(collectionName)
}
