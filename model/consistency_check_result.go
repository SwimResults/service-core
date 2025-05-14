package model

import "go.mongodb.org/mongo-driver/bson/primitive"

type ConsistencyType string

const (
	AthleteStartListTeamMissMatch ConsistencyType = "ATHLETE_START_LIST_TEAM_MISS_MATCH"
	AthleteChangedLastName        ConsistencyType = "ATHLETE_CHANGED_LAST_NAME"
	AthleteNameTypo               ConsistencyType = "ATHLETE_NAME_TYPO"
	AthleteNameMissing            ConsistencyType = "ATHLETE_NAME_MISSING"
	HeatTimeOrderViolation        ConsistencyType = "HEAT_TIME_ORDER_VIOLATION"
)

type ConsistencyCheckResults struct {
	Type      ConsistencyType    `json:"type"` // AthleteStartListTeamMissMatch, AthleteChangedLastName, AthleteNameTypo, AthleteNameMissing, HeatTimeOrderViolation
	Meeting   string             `json:"meeting"`
	ObjectId1 primitive.ObjectID `json:"object_id_1"`
	ObjectId2 primitive.ObjectID `json:"object_id_2"`
}

func GetConsistencyTypes() []ConsistencyType {
	return []ConsistencyType{AthleteStartListTeamMissMatch, AthleteChangedLastName, AthleteNameTypo, AthleteNameMissing, HeatTimeOrderViolation}
}
