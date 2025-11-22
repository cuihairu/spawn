#!/bin/bash

# API 测试脚本

BASE_URL="http://localhost:8888"

echo "=== Tappi 用户服务 API 测试 ==="

# 测试原有的测试接口
echo "1. 测试基础接口..."
curl -s "${BASE_URL}/from/you" | jq .

echo ""
echo "2. 测试用户注册..."
curl -s -X POST "${BASE_URL}/auth/register" \
  -H "Content-Type: application/json" \
  -d '{
    "username": "testuser",
    "email": "test@example.com",
    "password": "123456",
    "nickname": "测试用户"
  }' | jq .

echo ""
echo "3. 测试重复注册..."
curl -s -X POST "${BASE_URL}/auth/register" \
  -H "Content-Type: application/json" \
  -d '{
    "username": "testuser",
    "email": "test@example.com",
    "password": "123456"
  }' | jq .

echo ""
echo "4. 测试用户登录..."
curl -s -X POST "${BASE_URL}/auth/login" \
  -H "Content-Type: application/json" \
  -d '{
    "username": "testuser",
    "password": "123456"
  }' | jq .

echo ""
echo "5. 测试错误登录..."
curl -s -X POST "${BASE_URL}/auth/login" \
  -H "Content-Type: application/json" \
  -d '{
    "username": "testuser",
    "password": "wrongpassword"
  }' | jq .

echo ""
echo "6. 测试获取用户信息 (需要先登录获取token)..."
# 先登录获取 token
TOKEN_RESPONSE=$(curl -s -X POST "${BASE_URL}/auth/login" \
  -H "Content-Type: application/json" \
  -d '{
    "username": "testuser",
    "password": "123456"
  }')

TOKEN=$(echo $TOKEN_RESPONSE | jq -r '.token')
USER_ID=$(echo $TOKEN_RESPONSE | jq -r '.user_info.id')

echo "获取的 Token: $TOKEN"
echo "用户 ID: $USER_ID"

curl -s "${BASE_URL}/users/${USER_ID}" | jq .

echo ""
echo "=== 测试完成 ==="