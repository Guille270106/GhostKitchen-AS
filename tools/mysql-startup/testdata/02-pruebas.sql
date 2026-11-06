CREATE DATABASE integration_db;
CREATE USER 'prueba'@'%' IDENTIFIED BY 'test-prueba';
GRANT ALL PRIVILEGES ON integration_db.* TO 'prueba'@'%';
