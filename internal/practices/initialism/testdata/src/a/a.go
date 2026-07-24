package a

func GetUserId() string { return "" } // want `GetUserId uses "Id"; Go convention keeps initialisms like this fully uppercase \(ID\)`

func GetUserIDs() string { return "" }

func unexportedId() string { return "" }

type ApiClient struct{} // want `ApiClient uses "Api"; Go convention keeps initialisms like this fully uppercase \(API\)`
