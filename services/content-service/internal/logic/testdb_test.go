package logic

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/tappi/tappi/services/content-service/model"

	_ "github.com/mattn/go-sqlite3"
)

// seedContentStores 在临时文件 SQLite 中建表并写入给定种子，返回面向
// 接口的仓储（逻辑层测试沿用原内存仓的装载方式，仅换持久化底座）。
// 用文件库而非 :memory:：memory 库在多连接池下各连接各得一个空库，
// 文件库配合模型层单连接钳制行为与生产路径一致。
func seedContentStores(t *testing.T, guides []*model.Guide, comments []*model.Comment) (model.GuideStore, model.CommentStore) {
	t.Helper()

	db, err := sql.Open("sqlite3", "file:"+filepath.Join(t.TempDir(), "content.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	guideModel := model.NewGuideModel(db)
	if err := guideModel.CreateGuidesTable(); err != nil {
		t.Fatalf("create guides table: %v", err)
	}
	if err := guideModel.Seed(guides); err != nil {
		t.Fatalf("seed guides: %v", err)
	}

	commentModel := model.NewCommentModel(db)
	if err := commentModel.CreateCommentsTable(); err != nil {
		t.Fatalf("create comments table: %v", err)
	}
	if err := commentModel.Seed(comments); err != nil {
		t.Fatalf("seed comments: %v", err)
	}

	return guideModel, commentModel
}
