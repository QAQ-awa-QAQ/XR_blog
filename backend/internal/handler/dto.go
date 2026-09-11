package handler

import "personal_blog/internal/model"

// UserDTO 是下发给客户端的用户信息。
// 注意：不含密码哈希——3.3 要求的"密码"只以哈希形式留在数据库。
type UserDTO struct {
	ID       uint   `json:"id"`
	Account  string `json:"account"`
	Nickname string `json:"nickname"`
	Role     string `json:"role"`
	IP       string `json:"ip"`
}

func toUserDTO(u *model.User) UserDTO {
	if u == nil {
		return UserDTO{}
	}
	return UserDTO{
		ID:       u.ID,
		Account:  u.Account,
		Nickname: u.Nickname,
		Role:     u.Role,
		IP:       u.LastIP,
	}
}

func toUserDTOs(users []model.User) []UserDTO {
	out := make([]UserDTO, 0, len(users))
	for i := range users {
		out = append(out, toUserDTO(&users[i]))
	}
	return out
}
