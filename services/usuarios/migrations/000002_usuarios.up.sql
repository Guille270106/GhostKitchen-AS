-- G5: cuentas de Usuarios. Una fila por persona; el rol decide qué columnas aplican.
-- El correo se guarda en minúsculas y es único. La comparación del CHECK es binaria: con la
-- collation de la base, 'Ana@x.com' = 'ana@x.com' daría verdadero y el CHECK no frenaría nada.
-- La contraseña nunca se guarda: solo su hash bcrypt (60 caracteres).
CREATE TABLE usuario (
 id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 nombre VARCHAR(120) NOT NULL,
 email VARCHAR(254) NOT NULL,
 password_hash CHAR(60) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 direccion VARCHAR(255) NULL,
 rol VARCHAR(10) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 -- Cliente: la cocina que eligió la última vez (la puede cambiar).
 cocina_elegida_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL,
 -- Cuenta Cocina: la cocina que administra (fija).
 cocina_asignada_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL,
 -- Admin: la localidad que administra.
 localidad VARCHAR(80) NULL,
 activo BOOLEAN NOT NULL DEFAULT TRUE,
 creado_en TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
 actualizado_en TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
 CONSTRAINT uq_usuario_email UNIQUE (email),
 CONSTRAINT chk_usuario_rol CHECK (rol IN ('cliente','cocina','admin')),
 CONSTRAINT chk_usuario_nombre CHECK (CHAR_LENGTH(TRIM(nombre)) > 0),
 CONSTRAINT chk_usuario_email CHECK (email COLLATE utf8mb4_bin = LOWER(email) AND email LIKE '_%@_%'),
 -- Cada rol lleva solo sus datos: la cuenta Cocina necesita su cocina y el admin su localidad.
 CONSTRAINT chk_usuario_cocina CHECK ((rol = 'cocina') = (cocina_asignada_id IS NOT NULL)),
 CONSTRAINT chk_usuario_admin CHECK ((rol = 'admin') = (localidad IS NOT NULL)),
 CONSTRAINT chk_usuario_elegida CHECK (rol = 'cliente' OR cocina_elegida_id IS NULL),
 INDEX idx_usuario_cocina_asignada (cocina_asignada_id)
) ENGINE=InnoDB;
