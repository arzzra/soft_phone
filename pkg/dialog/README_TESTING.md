# Комплексная стратегия тестирования для dialog пакета

## Обзор

Этот документ описывает комплексную стратегию тестирования критических исправлений в пакете dialog, направленную на проверку thread-safety, производительности и стабильности в production-like условиях.

## Критические исправления для тестирования

1. **Thread-safe updateState/State операции** - защита состояния диалога от race conditions
2. **Safe referSubscriptions операции** - безопасная работа с REFER подписками
3. **Safe Close() с sync.Once** - атомарное закрытие диалогов
4. **Sharded dialog map с 32 шардами** - масштабируемое хранение диалогов
5. **Efficient ID generator pool** - высокопроизводительная генерация уникальных ID

## Структура тестов

### 1. Unit тесты (thread_safety_test.go)

**Цель**: Проверка thread-safety основных операций

**Тесты**:
- `TestThreadSafeUpdateState` - concurrent чтение/запись состояния
- `TestThreadSafeReferSubscriptions` - concurrent операции с подписками
- `TestThreadSafeClose` - множественные вызовы Close()
- `TestConcurrentStateChangeCallbacks` - безопасность колбэков
- `TestMemoryLeakDetection` - обнаружение утечек памяти

**Запуск**:
```bash
go test -v -run TestThreadSafe ./pkg/dialog/
go test -race -run TestThreadSafe ./pkg/dialog/
```

### 2. Sharded Map тесты (sharded_map_test.go)

**Цель**: Проверка производительности и корректности sharded map

**Тесты**:
- `TestShardedMapBasicOperations` - основные операции
- `TestShardedMapConcurrentAccess` - concurrent доступ
- `TestShardedMapLoadDistribution` - равномерность распределения
- `TestShardedMapHashDistribution` - качество хэш-функции

**Запуск**:
```bash
go test -v -run TestShardedMap ./pkg/dialog/
go test -race -run TestShardedMap ./pkg/dialog/
go test -bench=BenchmarkShardedMap ./pkg/dialog/
```

### 3. ID Generator тесты (id_generator_test.go)

**Цель**: Проверка эффективности и безопасности генератора ID

**Тесты**:
- `TestIDGeneratorUniqueness` - уникальность ID
- `TestIDGeneratorConcurrent` - concurrent безопасность
- `TestIDGeneratorPoolStats` - статистика пула
- `TestIDGeneratorStressTest` - стресс-тест

**Запуск**:
```bash
go test -v -run TestIDGenerator ./pkg/dialog/
go test -race -run TestIDGenerator ./pkg/dialog/
go test -bench=BenchmarkIDGenerator ./pkg/dialog/
```

### 4. Concurrent Load тесты (concurrent_load_test.go)

**Цель**: Проверка работы с >1000 одновременными диалогами

**Тесты**:
- `TestHighLoadConcurrentDialogs` - 2000+ одновременных диалогов
- `TestConcurrentDialogOperations` - concurrent операции на диалоге
- `TestStackConcurrentOperations` - concurrent операции стека

**Запуск**:
```bash
go test -v -timeout=3m -run TestHighLoad ./pkg/dialog/
go test -race -timeout=3m -run TestConcurrent ./pkg/dialog/
```

### 5. Performance Benchmark тесты (performance_benchmark_test.go)

**Цель**: Измерение производительности и выявление узких мест

**Тесты**:
- `BenchmarkDialogLifecycle` - полный жизненный цикл диалога
- `BenchmarkShardedMapPerformance` - сравнение с обычной map
- `BenchmarkIDGeneratorThroughput` - пропускная способность генератора
- `TestPerformanceCharacteristics` - комплексная оценка производительности

**Запуск**:
```bash
go test -bench=. -benchmem ./pkg/dialog/
go test -bench=BenchmarkDialogLifecycle -count=5 ./pkg/dialog/
go test -v -run TestPerformanceCharacteristics ./pkg/dialog/
```

### 6. Race Condition тесты (race_condition_test.go)

**Цель**: Обнаружение race conditions под нагрузкой

**Тесты**:
- `TestRaceConditionsDetection` - комплексная проверка race conditions
- `TestDataRaceDetectionUnderLoad` - обнаружение под реальной нагрузкой

**Запуск**:
```bash
go test -race -v -run TestRaceConditions ./pkg/dialog/
go test -race -timeout=2m -run TestDataRaceDetectionUnderLoad ./pkg/dialog/
```

### 7. Integration Production тесты (integration_production_test.go)

**Цель**: Симуляция production-like условий

**Тесты**:
- `TestProductionSimulation` - симуляция высоконагруженного SIP сервера
- `TestStressFailureRecovery` - восстановление после сбоев
- `TestLongRunningStability` - долговременная стабильность

**Запуск**:
```bash
go test -v -timeout=5m -run TestProductionSimulation ./pkg/dialog/
go test -v -timeout=3m -run TestStressFailureRecovery ./pkg/dialog/
go test -v -timeout=2m -run TestLongRunningStability ./pkg/dialog/
```

## Автоматизированные скрипты запуска

### Полный набор тестов
```bash
#!/bin/bash
echo "=== Запуск полного набора тестов dialog пакета ==="

echo "1. Unit тесты thread-safety..."
go test -v -race -run TestThreadSafe ./pkg/dialog/

echo "2. Sharded map тесты..."
go test -v -race -run TestShardedMap ./pkg/dialog/

echo "3. ID Generator тесты..."
go test -v -race -run TestIDGenerator ./pkg/dialog/

echo "4. Concurrent load тесты..."
go test -v -race -timeout=3m -run TestHighLoad ./pkg/dialog/

echo "5. Race condition detection..."
go test -race -v -timeout=2m -run TestRaceConditions ./pkg/dialog/

echo "6. Production simulation..."
go test -v -timeout=5m -run TestProductionSimulation ./pkg/dialog/

echo "7. Performance benchmarks..."
go test -bench=. -benchmem -timeout=5m ./pkg/dialog/
```

### Быстрые тесты (для CI/CD)
```bash
#!/bin/bash
echo "=== Быстрые тесты для CI/CD ==="
go test -short -race -v ./pkg/dialog/
go test -short -bench=BenchmarkShardedMap ./pkg/dialog/
go test -short -bench=BenchmarkIDGenerator ./pkg/dialog/
```

### Стресс-тесты (для нагрузочного тестирования)
```bash
#!/bin/bash
echo "=== Стресс-тесты ==="
go test -race -timeout=10m -run TestHighLoadConcurrentDialogs ./pkg/dialog/
go test -race -timeout=5m -run TestStressFailureRecovery ./pkg/dialog/
go test -timeout=10m -run TestLongRunningStability ./pkg/dialog/
```

## Критерии успешности

### Thread Safety
- ✅ Нет race conditions при запуске с `-race`
- ✅ Корректная работа под concurrent нагрузкой
- ✅ Безопасное множественное закрытие диалогов
- ✅ Защищенные колбэки от паник

### Производительность  
- ✅ >10,000 операций/сек на sharded map
- ✅ >50,000 ID генераций/сек
- ✅ Обработка >1000 одновременных диалогов
- ✅ Равномерное распределение по шардам (отклонение <50%)

### Стабильность
- ✅ Нет утечек памяти при длительной работе  
- ✅ Успешная обработка >95% запросов
- ✅ Восстановление после паник в колбэках
- ✅ Graceful shutdown без зависания

### Memory & Resources
- ✅ Рост памяти <3x за час работы
- ✅ Частота GC <10 раз/минуту
- ✅ Очистка диалогов после завершения
- ✅ Корректное освобождение ресурсов

## Мониторинг и метрики

### Во время тестирования отслеживаем:
- Количество горутин
- Использование памяти (Alloc, Sys)
- Частота сборки мусора
- Количество активных диалогов
- Статистику sharded map (распределение по шардам)
- Hit rate генератора ID
- Количество panic recovery

### Критические пороги:
- Memory leak: рост памяти >300% за тест
- High GC frequency: >10 сборок/минуту  
- Poor distribution: >50% диалогов в 20% шардах
- Low hit rate: <80% попаданий в ID pool
- High error rate: >5% ошибок операций

## Интеграция с CI/CD

### GitHub Actions пример:
```yaml
- name: Run Dialog Package Tests
  run: |
    go test -short -race -v ./pkg/dialog/
    go test -bench=BenchmarkShardedMap -benchmem ./pkg/dialog/
    go test -timeout=2m -run TestProductionSimulation ./pkg/dialog/
```

### Pre-commit hooks:
```bash
#!/bin/bash
echo "Running dialog package safety checks..."
go test -race -run TestThreadSafe ./pkg/dialog/ || exit 1
go test -run TestShardedMapBasicOperations ./pkg/dialog/ || exit 1
go vet ./pkg/dialog/ || exit 1
```

## Troubleshooting

### Частые проблемы и решения:

1. **Race conditions detected**
   - Проверить все мьютексы
   - Убедиться в правильном порядке блокировок
   - Проверить atomic операции

2. **Memory leaks**  
   - Проверить закрытие каналов
   - Убедиться в очистке карт и слайсов
   - Проверить отмену контекстов

3. **Poor performance**
   - Проверить распределение по шардам
   - Оптимизировать hit rate ID generator
   - Уменьшить время блокировок

4. **Test timeouts**
   - Увеличить timeout для медленных машин
   - Уменьшить количество операций в тестах
   - Проверить deadlock'и

## Заключение

Данная стратегия тестирования обеспечивает комплексную проверку всех критических исправлений в пакете dialog. Тесты покрывают:

- ✅ Thread safety всех операций
- ✅ Производительность sharded map
- ✅ Эффективность ID generator  
- ✅ Стабильность под нагрузкой
- ✅ Production-like сценарии
- ✅ Восстановление после сбоев

Регулярное выполнение этих тестов гарантирует высокое качество и надежность dialog пакета в production среде.