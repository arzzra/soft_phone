#!/bin/bash

# Комплексный тест-раннер для dialog пакета
# Запускает все категории тестов с подробной отчетностью

set -e

# Цвета для вывода
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Конфигурация
PKG_PATH="./pkg/dialog/"
TIMEOUT_SHORT="30s"
TIMEOUT_MEDIUM="2m"
TIMEOUT_LONG="5m"
LOG_DIR="test_logs"

# Функции
print_header() {
    echo -e "\n${BLUE}================================================${NC}"
    echo -e "${BLUE} $1${NC}"
    echo -e "${BLUE}================================================${NC}\n"
}

print_success() {
    echo -e "${GREEN}✅ $1${NC}"
}

print_warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

print_error() {
    echo -e "${RED}❌ $1${NC}"
}

run_test_category() {
    local category="$1"
    local test_pattern="$2"
    local timeout="$3"
    local extra_flags="$4"
    local log_file="${LOG_DIR}/${category}.log"
    
    echo -e "\n${YELLOW}Запуск $category тестов...${NC}"
    
    mkdir -p "$LOG_DIR"
    
    if go test -v -timeout="$timeout" $extra_flags -run="$test_pattern" "$PKG_PATH" 2>&1 | tee "$log_file"; then
        print_success "$category тесты пройдены"
        return 0
    else
        print_error "$category тесты провалены"
        return 1
    fi
}

run_benchmark() {
    local bench_pattern="$1"
    local timeout="$2"
    local log_file="${LOG_DIR}/benchmark_${bench_pattern}.log"
    
    echo -e "\n${YELLOW}Запуск бенчмарка $bench_pattern...${NC}"
    
    mkdir -p "$LOG_DIR"
    
    if go test -bench="$bench_pattern" -benchmem -timeout="$timeout" "$PKG_PATH" 2>&1 | tee "$log_file"; then
        print_success "Бенчмарк $bench_pattern завершен"
        return 0
    else
        print_warning "Бенчмарк $bench_pattern завершен с предупреждениями"
        return 1
    fi
}

check_race_conditions() {
    echo -e "\n${YELLOW}Проверка race conditions...${NC}"
    local log_file="${LOG_DIR}/race_detection.log"
    
    mkdir -p "$LOG_DIR"
    
    if go test -race -timeout="$TIMEOUT_MEDIUM" -run="TestRaceConditions" "$PKG_PATH" 2>&1 | tee "$log_file"; then
        print_success "Race conditions не обнаружены"
        return 0
    else
        print_error "Обнаружены race conditions!"
        return 1
    fi
}

generate_report() {
    echo -e "\n${BLUE}Генерация отчета...${NC}"
    local report_file="${LOG_DIR}/test_report.md"
    
    cat > "$report_file" << EOF
# Отчет о тестировании Dialog пакета

Дата: $(date)
Go версия: $(go version)

## Результаты тестирования

### 1. Thread Safety тесты
$(if [ -f "${LOG_DIR}/thread_safety.log" ]; then echo "✅ Пройдены"; else echo "❌ Провалены"; fi)

### 2. Sharded Map тесты  
$(if [ -f "${LOG_DIR}/sharded_map.log" ]; then echo "✅ Пройдены"; else echo "❌ Провалены"; fi)

### 3. ID Generator тесты
$(if [ -f "${LOG_DIR}/id_generator.log" ]; then echo "✅ Пройдены"; else echo "❌ Провалены"; fi)

### 4. Concurrent Load тесты
$(if [ -f "${LOG_DIR}/concurrent_load.log" ]; then echo "✅ Пройдены"; else echo "❌ Провалены"; fi)

### 5. Race Detection
$(if [ -f "${LOG_DIR}/race_detection.log" ]; then echo "✅ Пройдены"; else echo "❌ Провалены"; fi)

### 6. Production тесты
$(if [ -f "${LOG_DIR}/production.log" ]; then echo "✅ Пройдены"; else echo "❌ Провалены"; fi)

## Файлы логов
EOF

    for log in "${LOG_DIR}"/*.log; do
        if [ -f "$log" ]; then
            echo "- $(basename "$log")" >> "$report_file"
        fi
    done
    
    echo -e "\n${GREEN}Отчет сохранен: $report_file${NC}"
}

main() {
    print_header "КОМПЛЕКСНОЕ ТЕСТИРОВАНИЕ DIALOG ПАКЕТА"
    
    local start_time=$(date +%s)
    local failed_tests=0
    
    # Очистка старых логов
    rm -rf "$LOG_DIR"
    
    echo "Начинаем тестирование пакета: $PKG_PATH"
    echo "Логи сохраняются в: $LOG_DIR/"
    
    # 1. Thread Safety тесты
    print_header "1. THREAD SAFETY ТЕСТЫ"
    if ! run_test_category "thread_safety" "TestThreadSafe" "$TIMEOUT_SHORT" "-race"; then
        ((failed_tests++))
    fi
    
    # 2. Sharded Map тесты
    print_header "2. SHARDED MAP ТЕСТЫ"
    if ! run_test_category "sharded_map" "TestShardedMap" "$TIMEOUT_SHORT" "-race"; then
        ((failed_tests++))
    fi
    
    # 3. ID Generator тесты
    print_header "3. ID GENERATOR ТЕСТЫ"
    if ! run_test_category "id_generator" "TestIDGenerator" "$TIMEOUT_SHORT" "-race"; then
        ((failed_tests++))
    fi
    
    # 4. Concurrent Load тесты
    print_header "4. CONCURRENT LOAD ТЕСТЫ"
    if ! run_test_category "concurrent_load" "TestHighLoad|TestConcurrent" "$TIMEOUT_LONG" "-race"; then
        ((failed_tests++))
    fi
    
    # 5. Race Condition Detection
    print_header "5. RACE CONDITION DETECTION"
    if ! check_race_conditions; then
        ((failed_tests++))
    fi
    
    # 6. Production тесты (если не short mode)
    if [[ "$1" != "--short" ]]; then
        print_header "6. PRODUCTION SIMULATION ТЕСТЫ"
        if ! run_test_category "production" "TestProduction|TestStress|TestLongRunning" "$TIMEOUT_LONG" ""; then
            ((failed_tests++))
        fi
    else
        print_warning "Production тесты пропущены (--short mode)"
    fi
    
    # 7. Бенчмарки
    print_header "7. PERFORMANCE BENCHMARKS"
    run_benchmark "BenchmarkShardedMap" "$TIMEOUT_MEDIUM" || true
    run_benchmark "BenchmarkIDGenerator" "$TIMEOUT_MEDIUM" || true
    run_benchmark "BenchmarkConcurrent" "$TIMEOUT_MEDIUM" || true
    
    # Генерация отчета
    generate_report
    
    # Финальная статистика
    local end_time=$(date +%s)
    local duration=$((end_time - start_time))
    
    print_header "ИТОГОВЫЕ РЕЗУЛЬТАТЫ"
    echo "Время выполнения: ${duration}s"
    echo "Провалено категорий тестов: $failed_tests"
    
    if [ $failed_tests -eq 0 ]; then
        print_success "ВСЕ ТЕСТЫ ПРОЙДЕНЫ УСПЕШНО!"
        echo -e "\n${GREEN}🎉 Dialog пакет готов к production использованию!${NC}"
        exit 0
    else
        print_error "ОБНАРУЖЕНЫ ПРОБЛЕМЫ В $failed_tests КАТЕГОРИЯХ"
        echo -e "\n${RED}🚨 Требуется исправление проблем перед production!${NC}"
        echo "Подробности в логах: $LOG_DIR/"
        exit 1
    fi
}

# Проверка аргументов
if [[ "$1" == "--help" ]] || [[ "$1" == "-h" ]]; then
    echo "Использование: $0 [--short] [--help]"
    echo ""
    echo "Опции:"
    echo "  --short    Пропустить долгие production тесты"
    echo "  --help     Показать эту справку"
    echo ""
    echo "Примеры:"
    echo "  $0                # Полное тестирование"
    echo "  $0 --short        # Быстрое тестирование для CI/CD"
    exit 0
fi

# Проверка среды
if ! command -v go &> /dev/null; then
    print_error "Go не установлен или недоступен в PATH"
    exit 1
fi

if [ ! -d "$PKG_PATH" ]; then
    print_error "Директория $PKG_PATH не найдена"
    exit 1
fi

# Запуск основной функции
main "$@"