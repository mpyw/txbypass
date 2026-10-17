module example.com/app

go 1.23

require gorm.io/gorm v1.31.2

replace gorm.io/gorm => ../src/gorm.io/gorm
