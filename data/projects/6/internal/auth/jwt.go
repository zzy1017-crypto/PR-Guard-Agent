package auth

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var jwtSecret = []byte("flash_sale_secret") // JWT密钥，实际项目中应从配置文件或环境变量加载

// Claims定义了JWT的载荷结构，包含用户ID和标准的注册声明
type Claims struct {
	UserID string `json:"user_id"`

	jwt.RegisteredClaims
}

// GenerateToken生成token(JWT令牌)，包含用户ID和过期时间
func GenerateToken(userID string) (string, error) {

	claims := Claims{

		UserID: userID,

		RegisteredClaims: jwt.RegisteredClaims{
			//设置过期时间为24小时
			ExpiresAt: jwt.NewNumericDate(
				time.Now().Add(24 * time.Hour),
			),
		},
	}

	//创建一个新的token(JWT令牌)，使用HS256签名方法，并将claims作为载荷
	token := jwt.NewWithClaims(
		jwt.SigningMethodHS256,
		claims,
	)

	//签名token，返回一个字符串形式的JWT令牌，如果签名失败则返回错误
	return token.SignedString(jwtSecret)
}

// ParseToken解析token(JWT令牌)，验证其有效性并提取claims
func ParseToken(tokenStr string) (*Claims, error) {

	token, err := jwt.ParseWithClaims(

		tokenStr,  //要解析的token字符串

		&Claims{},  //一个空的Claims对象，用于接收解析后的claims数据

		//提供一个函数来返回用于验证token的密钥，这里直接返回jwtSecret
		func(token *jwt.Token) (interface{}, error) {

			return jwtSecret, nil
		},
	)

	if err != nil {
		return nil, err
	}

	//类型判断，确保claims是我们定义的Claims类型，并且token有效
	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, err
}
