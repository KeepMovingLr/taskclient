package controller

import (
	"crypto/md5"
	"errors"
	"fmt"
	redisconn "github.com/KeepMovingLr/taskserver/cache"
	"github.com/KeepMovingLr/taskserver/constant"
	"github.com/KeepMovingLr/taskserver/dto"
	"github.com/KeepMovingLr/taskserver/utils"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"ray.li/entrytaskclient/connectionpool"
	constant2 "ray.li/entrytaskclient/constant"
	"ray.li/entrytaskclient/session"
	"ray.li/entrytaskclient/view"
	"strconv"
	"strings"
	"time"
)

type ResponseData struct {
	User    view.UserVO
	Success bool
}

func LoginWithoutRpc(w http.ResponseWriter, r *http.Request) {
	// prevent CSRF attack
	token := r.FormValue("token")
	if !tokenCheck(w, token) {
		return
	}

	tmpl := template.Must(template.ParseFiles("static/html/profile.html"))
	parameter := new(dto.UserDTO)
	userNameValue := r.FormValue("username")
	passwordValue := r.FormValue("password")
	parameter.UserName = userNameValue
	parameter.Password = passwordValue
	parameter.MethodName = constant.MethodLoginCheck
	myConn, conErr := connectionpool.Pool.Get()
	defer connectionpool.Pool.Return(myConn)
	if conErr != nil {
		log.Println("get conn error:" + conErr.Error())
		commonErrorResponse(w)
		return
	}
	conn := myConn.Conn
	conn.SetReadDeadline(time.Now().Add(time.Duration(5000) * time.Millisecond))
	sendError := utils.SendMsg(conn, parameter)
	if sendError != nil {
		log.Println("send failed", sendError)
		commonErrorResponse(w)
		return
	}

	var userResult dto.UserDTO
	readErr := utils.ReadMsg(conn, &userResult)
	switch {
	case readErr == io.EOF:
		log.Println("read message failed - this connection has been closed", readErr)
		conn.Close()
		myConn.Invalid = true
		return
	case readErr != nil:
		log.Println("read message failed", readErr)
		commonErrorResponse(w)
		return
	}

	if userResult.Success {
		setSessionAndCookie(w, userResult, userNameValue)
	}

	data := ResponseData{
		User:    view.UserVO{userResult.UserName, userResult.NickName, userResult.ProfileUrl, "", generateRandomToken()},
		Success: userResult.Success,
	}
	tmpl.Execute(w, data)
	return
}

func tokenCheck(w http.ResponseWriter, token string) bool {
	tokenFromRedis, err := redisconn.Get(token)
	if err != nil {
		log.Println("get token from redis error:" + err.Error())
		commonErrorResponse(w)
		return false
	}
	if tokenFromRedis == nil {
		log.Println("illegal request :" + err.Error())
		commonErrorResponse(w)
		return false
	}
	if string(tokenFromRedis) != constant2.TokenUnUse {
		log.Println("illegal request :" + err.Error())
		commonErrorResponse(w)
		return false
	}
	redisconn.Put(token, constant2.TokenUsed)
	return true
}

func commonErrorResponse(w http.ResponseWriter) {
	tmpl := template.Must(template.ParseFiles("static/html/error.html"))
	data := ResponseData{
		Success: false,
	}
	tmpl.Execute(w, data)
}

func setSessionAndCookie(w http.ResponseWriter, userResult dto.UserDTO, userNameValue string) {
	// set session
	sessionId := session.GenerateSessionId()
	redisconn.PutWithExp(sessionId, userNameValue, 1800)

	session_Cookie := http.Cookie{
		Name:    constant2.CookieSession,
		Value:   sessionId,
		Expires: time.Now().Add(time.Minute * 15),
	}
	nick_Cookie := http.Cookie{
		Name:    constant2.CookieNickName,
		Value:   userResult.NickName,
		Expires: time.Now().Add(time.Minute * 15),
	}
	url_Cookie := http.Cookie{
		Name:    constant2.CookieProfileUrl,
		Value:   userResult.ProfileUrl,
		Expires: time.Now().Add(time.Minute * 15),
	}
	http.SetCookie(w, &session_Cookie)
	http.SetCookie(w, &nick_Cookie)
	http.SetCookie(w, &url_Cookie)

}

/*
*
index method to deal with index
*/
func Index(w http.ResponseWriter, r *http.Request) {

	token := generateRandomToken()
	sessionCookie, _ := r.Cookie(constant2.CookieSession)

	if sessionCookie != nil {
		tmpl := template.Must(template.ParseFiles("static/html/profile.html"))
		sessionId := sessionCookie.Value
		userNameByte, error := redisconn.Get(sessionId)
		if error != nil {
			commonErrorResponse(w)
			return
		}
		sessionErr := sessionCheck(sessionId, string(userNameByte))
		if sessionErr != nil {
			commonErrorResponse(w)
			return
		}
		nickCookie, _ := r.Cookie("nickName")
		urlCookie, _ := r.Cookie("profileUrl")

		data := ResponseData{
			// login status check
			User:    view.UserVO{string(userNameByte), nickCookie.Value, urlCookie.Value, "", token},
			Success: true,
		}
		tmpl.Execute(w, data)
	} else {
		tmpl := template.Must(template.ParseFiles("static/html/index.html"))
		tmpl.Execute(w, token)
	}
}

func generateRandomToken() string {
	h := md5.New()
	io.WriteString(h, strconv.FormatInt(time.Now().Unix(), 10))
	io.WriteString(h, "ganraomaxxxxxxxxx")
	token := fmt.Sprintf("%x", h.Sum(nil))
	// store the token to redis
	redisconn.PutWithExp(token, "unUse", 1800)
	return token
}

func sessionCheck(sessionId string, userName string) error {
	userNameFromSession, _ := redisconn.Get(sessionId)
	if userNameFromSession == nil {
		// 未登录
		return errors.New("not log in")
	}
	if string(userNameFromSession) != userName {
		return errors.New("userName has been changed")
	}
	return nil
}

func uploadFile(w http.ResponseWriter, r *http.Request) (filePath string, err error) {
	userName := r.FormValue("username")
	filePath = "static/img/" + userName
	if !Exists(filePath) {
		os.Mkdir(filePath, os.ModePerm)
	}
	// handle upload file
	r.ParseMultipartForm(32 << 20)
	file, handler, err := r.FormFile("uploadfile")
	if err != nil {
		return "", err
	}
	defer file.Close()

	fileType, fileTypeErr := checkFileType(handler.Filename)
	if fileTypeErr != nil {
		return "", fileTypeErr
	}

	tempFile, temErr := ioutil.TempFile(filePath, "upload*."+fileType)
	if temErr != nil {
		return "", temErr
	}
	defer tempFile.Close()
	fileBytes, ioErr := ioutil.ReadAll(file)
	if temErr != nil {
		return "", ioErr
	}
	tempFile.Write(fileBytes)
	return tempFile.Name(), nil
}

func checkFileType(filename string) (string, error) {
	split := strings.Split(filename, ".")
	last := split[len(split)-1:]
	fileType := last[0]
	if !strings.Contains("jpg,png,gif", fileType) {
		return "", errors.New("File type is wrong")
	}
	return fileType, nil
}

func Exists(path string) bool {
	_, err := os.Stat(path) //os.Stat获取文件信息
	if err != nil {
		if os.IsExist(err) {
			return true
		}
		return false
	}
	return true
}

func UpdateNickName(w http.ResponseWriter, r *http.Request) {
	token := r.FormValue("token")
	if !tokenCheck(w, token) {
		return
	}
	session, _ := r.Cookie(constant2.CookieSession)
	if session == nil {
		Index(w, r)
		return
	}

	filePath, err := uploadFile(w, r)
	if err != nil {
		log.Println("upload File error", err)
		commonErrorResponse(w)
		return
	}

	tmpl := template.Must(template.ParseFiles("static/html/profile.html"))
	parameter := new(dto.UserDTO)
	parameter.UserName = r.FormValue("username")
	parameter.NickName = r.FormValue("nickname")
	parameter.ProfileUrl = filePath
	parameter.MethodName = constant.MethodUpdateUserInfo
	parameter.GoSessionId = session.Value

	myConn, conErr := connectionpool.Pool.Get()
	defer connectionpool.Pool.Return(myConn)
	if conErr != nil {
		log.Println("get conn error:" + conErr.Error())
		commonErrorResponse(w)
		return
	}
	conn := myConn.Conn

	sendError := utils.SendMsg(conn, parameter)
	if sendError != nil {
		log.Println("send failed")
		commonErrorResponse(w)
		return
	}
	var userResult dto.UserDTO
	readError := utils.ReadMsg(conn, &userResult)
	if readError != nil {
		log.Println("read failed")
		commonErrorResponse(w)
		return
	}

	data := ResponseData{
		User:    view.UserVO{userResult.UserName, userResult.NickName, userResult.ProfileUrl, "", generateRandomToken()},
		Success: userResult.Success,
	}
	// if success,update cookie
	if userResult.Success {
		// set cookie
		sessionCookie := http.Cookie{
			Name:    constant2.CookieSession,
			Value:   userResult.GoSessionId,
			Expires: time.Now().Add(time.Minute * 30),
		}
		nickCookie := http.Cookie{
			Name:    constant2.CookieNickName,
			Value:   userResult.NickName,
			Expires: time.Now().Add(time.Minute * 30),
		}
		urlCookie := http.Cookie{
			Name:    constant2.CookieProfileUrl,
			Value:   userResult.ProfileUrl,
			Expires: time.Now().Add(time.Minute * 30),
		}
		http.SetCookie(w, &sessionCookie)
		http.SetCookie(w, &nickCookie)
		http.SetCookie(w, &urlCookie)
	}
	log.Print(data)
	tmpl.Execute(w, data)
}
