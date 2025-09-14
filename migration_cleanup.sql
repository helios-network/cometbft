-- Script de nettoyage optionnel pour supprimer les anciens block_events
-- ⚠️ OPTIONNEL - Ne pas exécuter si vous n'êtes pas sûr

-- Pour PostgreSQL (si vous utilisez psql indexer)
DROP VIEW IF EXISTS block_events CASCADE;

-- Pour LevelDB (si vous utilisez kv indexer)  
-- Les clés avec préfixe "block_events" seront simplement ignorées
-- Pas besoin de suppression manuelle
