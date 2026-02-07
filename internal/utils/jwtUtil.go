package jwtutil

import (
	"awesomeProject/internal/common"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// getSecretKey 获取密钥
func getSecretKey() []byte {
	if common.MineConfig != nil {
		return []byte(common.MineConfig.JWT.SecretKey)
	}
	// 默认密钥（仅开发环境使用）
	return []byte("your-very-secret-key-32-bytes")
}

// getTokenExpireDuration 获取Token过期时间
func getTokenExpireDuration() time.Duration {
	if common.MineConfig != nil {
		return time.Duration(common.MineConfig.JWT.ExpireHours) * time.Hour
	}
	// 默认24小时
	return time.Hour * 24
}

// 通用：生成 Token，支持任意结构体
func GenerateToken(claims interface{}) (string, error) {
	// 1. 将结构体转换为 map[string]interface{}
	claimMap, err := structToMap(claims)
	if err != nil {
		return "", err
	}

	// 2. 添加标准声明（exp, iat, nbf）
	claimMap["exp"] = time.Now().Add(getTokenExpireDuration()).Unix()
	claimMap["iat"] = time.Now().Unix()
	claimMap["nbf"] = time.Now().Unix()

	// 3. 创建 Token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims(claimMap))

	// 4. 签名
	return token.SignedString(getSecretKey())
}

func ParseTokenToMap(tokenString string) (map[string]interface{}, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return getSecretKey(), nil
	})
	if err != nil {
		return nil, err
	}

	if !token.Valid {
		return nil, errors.New("token is invalid")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("token claims is invalid")
	}

	return claims, nil
}

func ParseToken(tokenString string, dest interface{}) error {

	if reflect.TypeOf(dest).Kind() != reflect.Ptr {
		return errors.New("dest must be a pointer to a struct")
	}
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return getSecretKey(), nil
	})

	if err != nil {
		return err
	}

	if !token.Valid {
		return errors.New("token 无效")
	}

	// 提取 MapClaims
	if claims, ok := token.Claims.(jwt.MapClaims); ok {
		// 将 MapClaims 转回结构体
		if err := mapToStruct(claims, dest); err != nil {
			return err
		}
		return nil
	}

	return errors.New("无法解析 token 声明")
}

// 将结构体转为 map[string]interface{}
func structToMap(obj interface{}) (map[string]interface{}, error) {
	val := reflect.ValueOf(obj)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct {
		return nil, errors.New("输入必须是结构体")
	}

	result := make(map[string]interface{})
	typ := val.Type()
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := typ.Field(i)
		jsonTag := fieldType.Tag.Get("json")
		if jsonTag == "" || jsonTag == "-" {
			continue
		}
		key := jsonTag
		result[key] = field.Interface()
	}
	return result, nil
}

// 将 map[string]interface{} 转回结构体
func mapToStruct(data map[string]interface{}, dest interface{}) error {
	val := reflect.ValueOf(dest)
	if val.Kind() != reflect.Ptr {
		return errors.New("dest 必须是指针")
	}
	val = val.Elem()
	if val.Kind() != reflect.Struct {
		return errors.New("dest 必须是指针指向结构体")
	}

	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := val.Type().Field(i)
		jsonTag := fieldType.Tag.Get("json")
		if jsonTag == "" || jsonTag == "-" {
			continue
		}

		if field.CanSet() {
			value, ok := data[jsonTag]
			if !ok {
				continue
			}

			// 自动类型转换
			switch field.Kind() {
			case reflect.String:
				if s, ok := value.(string); ok {
					field.SetString(s)
				}
			case reflect.Int, reflect.Int64:
				if n, ok := value.(float64); ok {
					field.SetInt(int64(n))
				}
			case reflect.Uint, reflect.Uint64:
				if n, ok := value.(float64); ok {
					field.SetUint(uint64(n))
				}
			case reflect.Bool:
				if b, ok := value.(bool); ok {
					field.SetBool(b)
				}
			case reflect.Float64:
				if f, ok := value.(float64); ok {
					field.SetFloat(f)
				}
			default:
				continue
			}
		}
	}
	return nil
}
func ValidateToken(tokenString string, dest interface{}) bool {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return getSecretKey(), nil
	})
	return err == nil && token.Valid && ParseToken(tokenString, dest) == nil
}
