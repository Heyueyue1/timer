// Code generated by Wire. DO NOT EDIT.

//go:generate go run github.com/google/wire/cmd/wire
//go:build !wireinject
// +build !wireinject

package main

import (
	"bytes"
	"context"
	"fmt"
	"github.com/BitofferHub/gateway/rpc"
	"github.com/gin-gonic/gin"
	kgin "github.com/go-kratos/gin"
	"strconv"

	//"github.com/go-kratos/kratos/contrib/registry/etcd/v2"
	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/transport"
	"github.com/go-kratos/kratos/v2/transport/http"
	//clientv3 "go.etcd.io/etcd/client/v3"
	"io/ioutil"
	"time"
	jwt "github.com/appleboy/gin-jwt/v2"

	//"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/BitofferHub/gateway/internal/biz"
	"github.com/BitofferHub/gateway/internal/conf"
	"github.com/BitofferHub/gateway/internal/data"
	"github.com/BitofferHub/gateway/internal/server"
	"github.com/BitofferHub/gateway/internal/service"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
)

// Injectors from wire.go:

// wireApp init kratos application.
func wireApp(confServer *conf.Server, conf conf.Bootstrap, logger log.Logger) (*kratos.App, func(), error) {
	dataData, cleanup, err := data.NewData(conf.Data, logger)
	if err != nil {
		return nil, nil, err
	}
	greeterRepo := data.NewGreeterRepo(dataData, logger)
	greeterUsecase := biz.NewGreeterUsecase(greeterRepo, logger)
	greeterService := service.NewGreeterService(greeterUsecase)
	grpcServer := server.NewGRPCServer(confServer, greeterService, logger)
	router := gin.Default()
	// 使用kratos中间件
	router.Use(kgin.Middlewares(recovery.Recovery(), customMiddleware))
	httpServer := server.NewHTTPServer(confServer, greeterService, logger)
	httpSvr2 := CreateHttpSvr(conf.Server)
	httpServer.HandlePrefix("/", router)
	fmt.Println(conf.Server.Http.Addr)
	app := newApp(logger, grpcServer, httpSvr2)
	return app, func() {
		cleanup()
	}, nil
}


//jwt中payload的数据
type User struct {
	UserName string
	UserID string
}

//用于接受登录的用户名与密码
type login struct {
	Username string `form:"username" json:"username" binding:"required"`
	Password string `form:"password" json:"password" binding:"required"`
}

var identityKey = "jwtid"

func GetAuthMiddleware() *jwt.GinJWTMiddleware{
	// the jwt middleware
	authMiddleware, err := jwt.New(&jwt.GinJWTMiddleware{
		Realm:            "test zone",          //标识
		SigningAlgorithm: "HS256",              //加密算法
		Key:              []byte("secret key"), //密钥
		Timeout:          time.Hour,
		MaxRefresh:       time.Hour,   //刷新最大延长时间
		IdentityKey:      identityKey, //指定cookie的id
		PayloadFunc: func(data interface{}) jwt.MapClaims { //负载，这里可以定义返回jwt中的payload数据
			fmt.Println("into payload here")
			if u, ok := data.(*User); ok {
				userData := rpc.GetUserInfoByName(u.UserName)
				return jwt.MapClaims{
					identityKey: userData.UserID,
				}
			}
			return jwt.MapClaims{}
		},
		IdentityHandler: func(c *gin.Context) interface{} {
			claims := jwt.ExtractClaims(c)
			return &User{
				UserID: claims[identityKey].(string),
			}
		},
		Authenticator: Authenticator, //在这里可以写我们的登录验证逻辑
		Authorizator: func(data interface{}, c *gin.Context) bool { //当用户通过token请求受限接口时，会经过这段逻辑
			if v, ok := data.(*User); ok && v.UserID != "" {
				c.Set("userID", v.UserID)
				return true
			}
			return false
		},
		Unauthorized: func(c *gin.Context, code int, message string) { //错误时响应
			c.JSON(code, gin.H{
				"code":    code,
				"message": message,
			})
		},
		// 指定从哪里获取token 其格式为："<source>:<name>" 如有多个，用逗号隔开
		TokenLookup:   "header: Authorization, query: token, cookie: jwt",
		TokenHeadName: "Bearer",
		TimeFunc:      time.Now,
	})

	if err != nil {
		log.Fatal("JWT Error:" + err.Error())
	}

	return authMiddleware
}


//InfoLog func infoLog
func InfoLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		beginTime := time.Now()
		// ***** 1. get request body ****** //
		body, _ := ioutil.ReadAll(c.Request.Body)
		c.Request.Body.Close() //  must close
		c.Request.Body = ioutil.NopCloser(bytes.NewBuffer(body))
		// ***** 2. set requestID for goroutine ctx ****** //
		//duration := float64(time.Since(beginTime)) / float64(time.Second)
		fmt.Printf("ReqPath[%s]-Duration[%v]\n", c.Request.URL.Path, time.Since(beginTime))
	}
}

func customMiddleware(handler middleware.Handler) middleware.Handler {
	return func(ctx context.Context, req interface{}) (reply interface{}, err error) {
		fmt.Println("pikaqiu in it")
		if tr, ok := transport.FromServerContext(ctx); ok {
			fmt.Println("operation:", tr.Operation())
			fmt.Println("pikaqiu in it")
		}
		reply, err = handler(ctx, req)
		return
	}
}


func CreateHttpSvr(c *conf.Server) *http.Server{
	router := gin.Default()
	// 使用kratos中间件
	//router.Use(kgin.Middlewares(recovery.Recovery(), customMiddleware))
	router.Use(InfoLog())

	authMiddleware := GetAuthMiddleware()
	router.POST("/login", authMiddleware.LoginHandler)
	router.Use(authMiddleware.MiddlewareFunc())
	router.Any("/bitstorm/*action", Forward)

	router.GET("/get_user_info", func(ctx *gin.Context) {
		userID, _ := ctx.Get("userID")
		userIDStr := userID.(string)
		fmt.Println("userIDStr is", userIDStr)
		userIDInt, _:= strconv.ParseInt(userIDStr, 10, 64)
		userName := rpc.GetUserInfo(userIDInt)
		fmt.Printf("userID %s get success \n", userID)
		if userID == "error" {
			// 返回kratos error
			kgin.Error(ctx, errors.Unauthorized("auth_error", "no authentication"))
		} else {
			ctx.JSON(200, map[string]string{"welcome": userName})
		}
	})
	httpSrv := http.NewServer(http.Address(c.Http.Addr))
	httpSrv.HandlePrefix("/", router)
	return httpSrv
}

func Authenticator(c *gin.Context) (interface{}, error) {
	var loginVals login
	if err := c.ShouldBind(&loginVals); err != nil {
		return "", jwt.ErrMissingLoginValues
	}
	userName := loginVals.Username
	password := loginVals.Password

	userData := rpc.GetUserInfoByName(userName)
	if userData.Pwd == password {
		return &User{
			UserName:  userName,
			UserID: userData.UserID,
		}, nil
	}
	return nil, jwt.ErrFailedAuthentication
}

/*
func getUserInfo(userID int64) string {
	client, err := clientv3.New(clientv3.Config{
		Endpoints: []string{"127.0.0.1:2379"},
	})
	if err != nil {
		panic(err)
	}
	// new dis with etcd client
	dis := etcd.New(client)
	endpoint := "discovery:///user-svr"
	connHTTP, err := http.NewClient(
		context.Background(),
		http.WithEndpoint(endpoint),
		http.WithDiscovery(dis),
		http.WithBlock(),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer connHTTP.Close()

	httpClient := pb.NewUserHTTPClient(connHTTP)
	fmt.Printf("before call\n")
	reply, err := httpClient.GetUser(context.Background(), &pb.GetUserRequest{UserID: 123})
	if err != nil {
		log.Fatal(err)
		return
	}
	fmt.Printf("[http] GetUser %+v\n", reply)
	time.Sleep(10 * time.Second)

}*/
